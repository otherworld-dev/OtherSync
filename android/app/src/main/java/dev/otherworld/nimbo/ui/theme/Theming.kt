/*
 * Theming.kt — the decisions behind the theme, kept pure.
 *
 * All three are made from values someone else controls: a colour a server admin
 * typed into Nextcloud, an appearance the user set there, and a preference they
 * set here. None of them can be trusted to be sensible, and none of them should
 * be able to produce an app you cannot read.
 */
package dev.otherworld.nimbo.ui.theme

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.toArgb

/** Where the app takes dark/light from. */
enum class AppearancePreference(val stored: String, val label: String) {
    /** The user's Nextcloud appearance, falling back to the phone. */
    FOLLOW_NEXTCLOUD("nextcloud", "Follow Nextcloud"),
    FOLLOW_SYSTEM("system", "Follow phone"),
    ALWAYS_DARK("dark", "Always dark"),
    ALWAYS_LIGHT("light", "Always light");

    companion object {
        /**
         * Reads a stored value. Anything unrecognised — a preference written by
         * a later version, a corrupted file — falls back to the default rather
         * than throwing on a value the user cannot see or fix.
         */
        fun fromStored(value: String?): AppearancePreference =
            entries.firstOrNull { it.stored == value } ?: FOLLOW_NEXTCLOUD
    }
}

/**
 * The server's theming colour, or null if it did not give us one we can use.
 *
 * Null matters: the caller keeps its own palette rather than substituting a
 * guess. A wrong accent is worse than no accent, because it can be illegible.
 */
fun parseThemeColor(hex: String?): Color? {
    val raw = hex?.trim()?.removePrefix("#") ?: return null
    val expanded = when (raw.length) {
        // #RGB shorthand, which Nextcloud accepts from an admin.
        3 -> raw.map { "$it$it" }.joinToString("")
        6 -> raw
        else -> return null
    }
    if (!expanded.all { it.isDigit() || it.lowercaseChar() in 'a'..'f' }) return null
    val value = expanded.toLongOrNull(16) ?: return null
    return Color(0xFF000000L or value)
}

/**
 * Black or white, whichever reads better on [background]: the one with the
 * higher WCAG contrast. (A plain brightness cut-off put white on dragon red at
 * 4.1:1 when black gives 5.4:1.) Either always reaches at least 4.5:1.
 */
fun readableOn(background: Color): Color =
    Color(readableOnArgb(background.toArgb().toLong() and 0xFFFFFFFFL))

/**
 * [readableOn] for a packed 0xAARRGGBB colour. Pure, so the unit tests can
 * hold the supporter accents to exactly the same rule without Compose.
 */
fun readableOnArgb(argb: Long): Long {
    val l = relativeLuminance(argb)
    val onWhite = 1.05 / (l + 0.05)
    val onBlack = (l + 0.05) / 0.05
    return if (onBlack > onWhite) 0xFF000000 else 0xFFFFFFFF
}

/** WCAG relative luminance of a packed 0xAARRGGBB colour. */
fun relativeLuminance(argb: Long): Double {
    fun channel(shift: Int): Double {
        val c = ((argb shr shift) and 0xFF) / 255.0
        return if (c <= 0.04045) c / 12.92 else Math.pow((c + 0.055) / 1.055, 2.4)
    }
    return 0.2126 * channel(16) + 0.7152 * channel(8) + 0.0722 * channel(0)
}

/** WCAG contrast ratio between two packed colours (1 to 21). */
fun contrastRatio(a: Long, b: Long): Double {
    val la = relativeLuminance(a)
    val lb = relativeLuminance(b)
    return (maxOf(la, lb) + 0.05) / (minOf(la, lb) + 0.05)
}

/** The backgrounds an accent has to stand out on (Theme.kt's surfaces). */
const val LIGHT_SURFACE: Long = 0xFFF4F0E8 // chalk
const val DARK_SURFACE: Long = 0xFF23262B // slate

/** Where the accent comes from when no supporter colour is chosen. */
enum class AccentSource(val stored: String, val label: String) {
    /** OtherSync's own valley green (Theme.kt's palette). */
    BRAND("brand", "OtherSync colour"),
    /** The Nextcloud server's theme colour, as the app did before the rebrand. */
    SERVER("server", "Match Nextcloud theme colour");

    companion object {
        fun fromStored(value: String?): AccentSource =
            entries.firstOrNull { it.stored == value } ?: BRAND
    }
}

/**
 * Makes any colour usable as an accent on one theme: keeps its hue and
 * saturation and moves its lightness (darker on the light theme, lighter on
 * the dark one) just far enough to stand 3:1 off the surface. Text on it then
 * takes [readableOnArgb], which always manages 4.5:1. Used for a supporter's
 * custom colour, which can be anything at all.
 */
fun fitAccentArgb(argb: Long, dark: Boolean): Long {
    val surface = if (dark) DARK_SURFACE else LIGHT_SURFACE
    if (contrastRatio(argb, surface) >= 3.0) return argb or 0xFF000000
    val (h, sat, light) = toHsl(argb)
    var l = light
    repeat(100) {
        l = if (dark) minOf(1.0, l + 0.01) else maxOf(0.0, l - 0.01)
        val candidate = fromHsl(h, sat, l)
        if (contrastRatio(candidate, surface) >= 3.0) return candidate
    }
    return if (dark) 0xFFFFFFFF else 0xFF000000
}

private fun toHsl(argb: Long): Triple<Double, Double, Double> {
    val r = ((argb shr 16) and 0xFF) / 255.0
    val g = ((argb shr 8) and 0xFF) / 255.0
    val b = (argb and 0xFF) / 255.0
    val max = maxOf(r, g, b)
    val min = minOf(r, g, b)
    val l = (max + min) / 2
    if (max == min) return Triple(0.0, 0.0, l)
    val d = max - min
    val s = if (l > 0.5) d / (2 - max - min) else d / (max + min)
    val h = when (max) {
        r -> ((g - b) / d + (if (g < b) 6 else 0)) / 6
        g -> ((b - r) / d + 2) / 6
        else -> ((r - g) / d + 4) / 6
    }
    return Triple(h, s, l)
}

private fun fromHsl(h: Double, s: Double, l: Double): Long {
    fun hue(p: Double, q: Double, tIn: Double): Double {
        var t = tIn
        if (t < 0) t += 1.0
        if (t > 1) t -= 1.0
        return when {
            t < 1.0 / 6 -> p + (q - p) * 6 * t
            t < 1.0 / 2 -> q
            t < 2.0 / 3 -> p + (q - p) * (2.0 / 3 - t) * 6
            else -> p
        }
    }
    val (r, g, b) = if (s == 0.0) Triple(l, l, l) else {
        val q = if (l < 0.5) l * (1 + s) else l + s - l * s
        val p = 2 * l - q
        Triple(hue(p, q, h + 1.0 / 3), hue(p, q, h), hue(p, q, h - 1.0 / 3))
    }
    fun byte(v: Double) = Math.round(v.coerceIn(0.0, 1.0) * 255).toLong()
    return 0xFF000000L or (byte(r) shl 16) or (byte(g) shl 8) or byte(b)
}

/**
 * The accent to apply, or null for the theme's own valley-green palette.
 * A supporter's chosen colour wins; otherwise the free setting decides
 * between OtherSync's colour and the server's.
 */
fun resolveAccentArgb(supporterArgb: Long?, source: AccentSource, serverColor: Color?): Long? =
    supporterArgb ?: if (source == AccentSource.SERVER) serverColor?.toArgb()?.toLong()?.and(0xFFFFFFFFL) else null

/**
 * Whether to render dark.
 *
 * [serverAppearance] is what Nextcloud reports: "dark", "light", or "default"
 * (meaning the user told Nextcloud to follow their own OS). "default", unknown
 * values and no answer at all are the same thing here — the server has no
 * opinion, so the phone decides.
 */
fun resolveDark(
    preference: AppearancePreference,
    serverAppearance: String?,
    systemDark: Boolean,
): Boolean = when (preference) {
    AppearancePreference.ALWAYS_DARK -> true
    AppearancePreference.ALWAYS_LIGHT -> false
    AppearancePreference.FOLLOW_SYSTEM -> systemDark
    AppearancePreference.FOLLOW_NEXTCLOUD -> when (serverAppearance?.trim()?.lowercase()) {
        "dark" -> true
        "light" -> false
        else -> systemDark
    }
}
