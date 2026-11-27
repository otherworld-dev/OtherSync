package dev.otherworld.nimbo.supporter

import dev.otherworld.nimbo.ui.theme.readableOnArgb
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import kotlin.math.pow

class CosmeticsTest {

    @Test
    fun `a lapsed supporter sees the defaults but keeps their choices`() {
        val lapsed = Perks.of(SupporterTier.NONE)
        assertEquals(AppIcon.DEFAULT, effectiveIcon(AppIcon.EMBER, lapsed))
        assertEquals(AccentChoice.FOLLOW_NEXTCLOUD, effectiveAccent(AccentChoice.ESTUARY, lapsed))

        val ui = supporterUi(SupporterStatus(), null, SupporterChoices(icon = AppIcon.EMBER, accent = AccentChoice.ESTUARY))
        assertEquals(AppIcon.EMBER, ui.icon)
        assertEquals(AppIcon.DEFAULT, ui.effectiveIcon)
        assertEquals(AccentChoice.ESTUARY, ui.accent)
        assertEquals(AccentChoice.FOLLOW_NEXTCLOUD, ui.effectiveAccent)
    }

    @Test
    fun `resubscribing brings the choices straight back`() {
        assertEquals(AppIcon.EMBER, effectiveIcon(AppIcon.EMBER, Perks.of(SupporterTier.BACKER)))
        assertEquals(AccentChoice.ESTUARY, effectiveAccent(AccentChoice.ESTUARY, Perks.of(SupporterTier.PATRON)))
        // A Backer has icons but not accents.
        assertEquals(AccentChoice.FOLLOW_NEXTCLOUD, effectiveAccent(AccentChoice.ESTUARY, Perks.of(SupporterTier.BACKER)))
    }

    @Test
    fun `stored names that no longer exist fall back to the defaults`() {
        assertEquals(AppIcon.DEFAULT, AppIcon.fromStored("RAINBOW"))
        assertEquals(AppIcon.DEFAULT, AppIcon.fromStored(null))
        assertEquals(AccentChoice.FOLLOW_NEXTCLOUD, AccentChoice.fromStored("GOLD"))
    }

    @Test
    fun `Nimbo-era colours load as their nearest family colour`() {
        // SupporterPrefs stores the enum name; an old choice must not reset to nothing.
        assertEquals(AccentChoice.DAMSON, AccentChoice.fromStored("INDIGO"))
        assertEquals(AccentChoice.DAMSON, AccentChoice.fromStored("VIOLET"))
        assertEquals(AccentChoice.ESTUARY, AccentChoice.fromStored("TEAL"))
        assertEquals(AccentChoice.SLATE_BLUE, AccentChoice.fromStored("OCEAN"))
        assertEquals(AccentChoice.DRAGON, AccentChoice.fromStored("CRIMSON"))
        // Forest green is OtherSync's own colour now.
        assertEquals(AccentChoice.FOLLOW_NEXTCLOUD, AccentChoice.fromStored("FOREST"))
    }

    @Test
    fun `every current accent name round-trips`() {
        for (choice in AccentChoice.entries) {
            assertEquals(choice, AccentChoice.fromStored(choice.name))
        }
    }

    @Test
    fun `a custom colour applies only once one is saved`() {
        assertNull(supporterAccentArgb(AccentChoice.CUSTOM, null, dark = false))
        assertEquals(0xFFB3262DL, supporterAccentArgb(AccentChoice.CUSTOM, 0xFFB3262DL, dark = false))
        assertNull(supporterAccentArgb(AccentChoice.FOLLOW_NEXTCLOUD, 0xFFB3262DL, dark = true))
        assertEquals(0xFFDA4D3FL, supporterAccentArgb(AccentChoice.DRAGON, null, dark = true))
    }

    // WCAG relative luminance — the yardstick the fixed accents are held to.
    private fun channel(argb: Long, shift: Int): Double {
        val c = ((argb shr shift) and 0xFF) / 255.0
        return if (c <= 0.04045) c / 12.92 else ((c + 0.055) / 1.055).pow(2.4)
    }

    private fun luminance(argb: Long) =
        0.2126 * channel(argb, 16) + 0.7152 * channel(argb, 8) + 0.0722 * channel(argb, 0)

    private fun contrast(a: Long, b: Long): Double {
        val (hi, lo) = listOf(luminance(a), luminance(b)).sortedDescending()
        return (hi + 0.05) / (lo + 0.05)
    }

    @Test
    fun `every fixed accent is readable under its own text colour, on both themes`() {
        for (accent in AccentChoice.entries) {
            for (dark in listOf(false, true)) {
                val argb = accent.argbFor(dark) ?: continue
                val ratio = contrast(argb, readableOnArgb(argb))
                assertTrue("${accent.label} dark=$dark: $ratio", ratio >= 4.5)
            }
        }
    }

    @Test
    fun `every fixed accent stands out on its theme's surface`() {
        val lightSurface = 0xFFF4F0E8 // chalk
        val darkSurface = 0xFF23262B // slate
        for (accent in AccentChoice.entries) {
            accent.argbFor(false)?.let { assertTrue("${accent.label} on light", contrast(it, lightSurface) >= 3.0) }
            accent.argbFor(true)?.let { assertTrue("${accent.label} on dark", contrast(it, darkSurface) >= 3.0) }
        }
    }
}
