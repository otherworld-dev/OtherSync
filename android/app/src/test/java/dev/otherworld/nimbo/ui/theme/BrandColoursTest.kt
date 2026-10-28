package dev.otherworld.nimbo.ui.theme

import androidx.compose.material3.ColorScheme
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.toArgb
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import kotlin.math.pow

/**
 * The app's own colours, used before sign-in or when the server gives none.
 * Held to the same WCAG bars as the supporter accents (CosmeticsTest): 4.5:1
 * for text on a colour, 3:1 for the accent against the surface it sits on.
 */
class BrandColoursTest {

    private fun channel(argb: Int, shift: Int): Double {
        val c = ((argb shr shift) and 0xFF) / 255.0
        return if (c <= 0.04045) c / 12.92 else ((c + 0.055) / 1.055).pow(2.4)
    }

    private fun luminance(c: Color): Double {
        val argb = c.toArgb()
        return 0.2126 * channel(argb, 16) + 0.7152 * channel(argb, 8) + 0.0722 * channel(argb, 0)
    }

    private fun contrast(a: Color, b: Color): Double {
        val (hi, lo) = listOf(luminance(a), luminance(b)).sortedDescending()
        return (hi + 0.05) / (lo + 0.05)
    }

    private val schemes: Map<String, ColorScheme> = mapOf("light" to LightColors, "dark" to DarkColors)

    @Test
    fun `the accent is OtherSync teal`() {
        assertEquals(Color(0xFF22D3C4), BrandTeal)
        assertEquals(Color(0xFF0D9488), BrandDeepTeal)
        assertEquals(BrandDeepTeal, LightColors.primary)
        assertEquals(BrandTeal, DarkColors.primary)
    }

    @Test
    fun `text on the accent and on its containers is readable`() {
        for ((name, s) in schemes) {
            val pairs = mapOf(
                "onPrimary" to (s.primary to s.onPrimary),
                "onPrimaryContainer" to (s.primaryContainer to s.onPrimaryContainer),
                "onSecondaryContainer" to (s.secondaryContainer to s.onSecondaryContainer),
            )
            for ((role, pair) in pairs) {
                val ratio = contrast(pair.first, pair.second)
                assertTrue("$name $role: $ratio", ratio >= 4.5)
            }
        }
    }

    @Test
    fun `the accent stands out on the surface it sits on`() {
        for ((name, s) in schemes) {
            val onSurface = contrast(s.primary, s.surface)
            assertTrue("$name primary on surface: $onSurface", onSurface >= 3.0)
            val inverse = contrast(s.inversePrimary, s.inverseSurface)
            assertTrue("$name inversePrimary on inverseSurface: $inverse", inverse >= 3.0)
        }
    }
}
