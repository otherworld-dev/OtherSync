/*
 * Theme.kt — the Material 3 theme for the whole app.
 *
 * Wallpaper-based dynamic colour is deliberately off. The accent instead comes
 * from the user's own Nextcloud theme, exactly as the desktop client does — the
 * app should look like their server, not like their wallpaper. The schemes below
 * are the fallback for before sign-in, offline, or a server colour we cannot
 * read.
 */
package dev.otherworld.nimbo.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.compositeOver

/*
 * The OtherSync brand accent (Other family brief, 15 Nov 2026): valley green on
 * dark surfaces, deep valley green on light ones, over the Cŵn Annwn studio
 * neutrals (slate, chalk, stone) rather than Material's generated greys. White
 * reads on the deep green (6.4:1); on the brighter dark-mode green it is only
 * 3.2:1, so slate sits on it instead (4.8:1).
 */

/** OtherSync valley green (#5FA05B): the accent on dark surfaces. */
val BrandGreen = Color(0xFF5FA05B)

/** Deep valley green (#2F6B3A): the accent on light surfaces. */
val BrandDeepGreen = Color(0xFF2F6B3A)

/** Slate (#23262B): dark backgrounds, text on light, text on the bright green. */
val BrandSlate = Color(0xFF23262B)

/** Chalk (#F4F0E8): light backgrounds and text on dark. */
val BrandChalk = Color(0xFFF4F0E8)

/** Stone (#ECE6DB): light panels. */
val BrandStone = Color(0xFFECE6DB)

internal val LightColors = lightColorScheme(
    primary = BrandDeepGreen,
    onPrimary = Color(0xFFFFFFFF),
    primaryContainer = Color(0xFFD3E6CF),
    onPrimaryContainer = Color(0xFF0F2A14),
    secondary = Color(0xFF5A5650),
    onSecondary = Color(0xFFFFFFFF),
    secondaryContainer = BrandStone,
    onSecondaryContainer = BrandSlate,
    tertiary = Color(0xFF56632F),
    onTertiary = Color(0xFFFFFFFF),
    tertiaryContainer = Color(0xFFDFE6C6),
    onTertiaryContainer = Color(0xFF1C2208),
    error = Color(0xFFA63A2B),
    onError = Color(0xFFFFFFFF),
    errorContainer = Color(0xFFF6DAD3),
    onErrorContainer = Color(0xFF410E06),
    background = BrandChalk,
    onBackground = BrandSlate,
    surface = BrandChalk,
    onSurface = BrandSlate,
    surfaceVariant = BrandStone,
    onSurfaceVariant = Color(0xFF5A5650),
    surfaceTint = BrandDeepGreen,
    outline = Color(0xFF8F897F),
    outlineVariant = Color(0xFFD5CEC1),
    inverseSurface = BrandSlate,
    inverseOnSurface = BrandChalk,
    // Snackbar actions sit on the slate inverse surface, so the bright green.
    inversePrimary = BrandGreen,
    scrim = Color(0xFF000000),
    surfaceBright = Color(0xFFFAF8F3),
    surfaceDim = Color(0xFFDDD6C9),
    surfaceContainerLowest = Color(0xFFFFFFFF),
    surfaceContainerLow = Color(0xFFF8F5EF),
    surfaceContainer = Color(0xFFF0EBE2),
    surfaceContainerHigh = BrandStone,
    surfaceContainerHighest = Color(0xFFE5DED1),
)

internal val DarkColors = darkColorScheme(
    primary = BrandGreen,
    onPrimary = BrandSlate,
    primaryContainer = Color(0xFF2C4A2E),
    onPrimaryContainer = Color(0xFFD3E6CF),
    secondary = Color(0xFFA8A39A),
    onSecondary = BrandSlate,
    secondaryContainer = Color(0xFF3A3D42),
    onSecondaryContainer = BrandStone,
    tertiary = Color(0xFFA3B46B),
    onTertiary = BrandSlate,
    tertiaryContainer = Color(0xFF3E4720),
    onTertiaryContainer = Color(0xFFDFE6C6),
    error = Color(0xFFE58B74),
    onError = BrandSlate,
    errorContainer = Color(0xFF7A2A1F),
    onErrorContainer = Color(0xFFF6DAD3),
    background = BrandSlate,
    onBackground = BrandChalk,
    surface = BrandSlate,
    onSurface = BrandChalk,
    surfaceVariant = Color(0xFF3A3D42),
    onSurfaceVariant = Color(0xFFC9C3B8),
    surfaceTint = BrandGreen,
    outline = Color(0xFF8F8A82),
    outlineVariant = Color(0xFF3A3D42),
    inverseSurface = BrandChalk,
    inverseOnSurface = BrandSlate,
    // On the chalk inverse surface the bright green is only 2.8:1, so the
    // deep green (5.6:1).
    inversePrimary = BrandDeepGreen,
    scrim = Color(0xFF000000),
    surfaceBright = Color(0xFF3A3D42),
    surfaceDim = Color(0xFF1E2124),
    surfaceContainerLowest = Color(0xFF1B1E21),
    surfaceContainerLow = Color(0xFF26292E),
    surfaceContainer = Color(0xFF2A2D32),
    surfaceContainerHigh = Color(0xFF303338),
    surfaceContainerHighest = Color(0xFF383B40),
)

/** Material 3 default type scale — no custom fonts in the walking skeleton. */
private val NimboTypography = Typography()

@Composable
fun NimboTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    /**
     * The user's Nextcloud theme colour, as the desktop client uses it. Null —
     * before sign-in, offline, or when the server reports something unusable —
     * keeps the palette below, which is never wrong, only generic.
     */
    accent: Color? = null,
    content: @Composable () -> Unit,
) {
    val base = if (darkTheme) DarkColors else LightColors
    // Only the primary roles are re-pointed. Deriving a whole palette from one
    // hex needs the HCT maths in material-color-utilities, which this project's
    // pinned dependency set does not include — and the desktop does the same
    // thing anyway: it emits ONE accent, not a generated scheme.
    val scheme = if (accent == null) base else base.copy(
        primary = accent,
        onPrimary = readableOn(accent),
        // The container is the same hue softened toward the surface, so it
        // still reads as "this app's colour" without competing with the accent.
        primaryContainer = accent.copy(alpha = if (darkTheme) 0.32f else 0.18f)
            .compositeOver(base.surface),
        onPrimaryContainer = base.onSurface,
    )
    MaterialTheme(
        colorScheme = scheme,
        typography = NimboTypography,
        content = content,
    )
}
