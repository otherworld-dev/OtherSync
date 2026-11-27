/*
 * Cosmetics.kt — the extras supporters can choose, and what is actually shown.
 *
 * A choice is remembered even after its perk lapses, so resubscribing brings
 * it straight back. What is APPLIED is always worked out from the perks.
 */
package dev.otherworld.nimbo.supporter

import dev.otherworld.nimbo.ui.theme.fitAccentArgb

/** One launcher alias per icon (see AndroidManifest.xml). */
enum class AppIcon(val aliasName: String, val label: String) {
    DEFAULT("dev.otherworld.nimbo.LauncherDefault", "OtherSync"),
    FOREST("dev.otherworld.nimbo.LauncherForest", "Forest"),
    EMBER("dev.otherworld.nimbo.LauncherEmber", "Ember"),
    SLATE("dev.otherworld.nimbo.LauncherSlate", "Slate");

    companion object {
        fun fromStored(value: String?): AppIcon = entries.firstOrNull { it.name == value } ?: DEFAULT
    }
}

/**
 * The supporter accent colours: the Other family palette (rebrand brief, Nov
 * 2026), each with the shade it wears on a light theme and on a dark one, plus
 * [CUSTOM], the supporter's own colour (stored in SupporterPrefs and fitted to
 * stay readable by fitAccentArgb). [FOLLOW_NEXTCLOUD] means "no supporter
 * colour": the free Accent colour setting decides, OtherSync green or the
 * server's.
 *
 * The enum NAMES are what SupporterPrefs saves, so never rename one. The
 * Nimbo-era colours (Indigo, Teal, ...) are no longer offered; a saved one
 * loads as its nearest family colour (see [fromStored]).
 */
enum class AccentChoice(val light: Long?, val dark: Long?, val label: String) {
    FOLLOW_NEXTCLOUD(null, null, "Default"),
    DAMSON(0xFF5B468F, 0xFF8C76C4, "Damson"),
    ESTUARY(0xFF2D6767, 0xFF4E9A9A, "Estuary teal"),
    SLATE_BLUE(0xFF34597F, 0xFF6A92BC, "Slate blue"),
    DRAGON(0xFFB3262D, 0xFFDA4D3F, "Dragon red"),
    GORSE(0xFF9A4A14, 0xFFD9773C, "Gorse orange"),
    BRACKEN(0xFF9E5F1B, 0xFFD89B3E, "Bracken amber"),
    LICHEN(0xFF6A5D16, 0xFFC9B44A, "Lichen"),
    CUSTOM(null, null, "Custom");

    /** The fixed colour for one theme, or null for [FOLLOW_NEXTCLOUD] and [CUSTOM]. */
    fun argbFor(dark: Boolean): Long? = if (dark) this.dark else light

    companion object {
        /** Nimbo-era names, kept loading as the nearest family colour. */
        private val LEGACY = mapOf(
            "INDIGO" to DAMSON,
            "VIOLET" to DAMSON,
            "TEAL" to ESTUARY,
            "OCEAN" to SLATE_BLUE,
            "CRIMSON" to DRAGON,
            // Forest green is OtherSync's own colour now.
            "FOREST" to FOLLOW_NEXTCLOUD,
        )

        fun fromStored(value: String?): AccentChoice =
            entries.firstOrNull { it.name == value } ?: LEGACY[value] ?: FOLLOW_NEXTCLOUD
    }
}

/**
 * The supporter colour to apply on one theme, or null when there isn't one
 * (no perk, "Default", or Custom with nothing saved yet). A custom colour is
 * fitted per theme so it always stands out and carries readable text.
 */
fun supporterAccentArgb(effective: AccentChoice, custom: Long?, dark: Boolean): Long? = when (effective) {
    AccentChoice.CUSTOM -> custom?.let { fitAccentArgb(it, dark) }
    else -> effective.argbFor(dark)
}

fun effectiveIcon(chosen: AppIcon, perks: Perks): AppIcon = if (perks.icons) chosen else AppIcon.DEFAULT

fun effectiveAccent(chosen: AccentChoice, perks: Perks): AccentChoice =
    if (perks.accents) chosen else AccentChoice.FOLLOW_NEXTCLOUD
