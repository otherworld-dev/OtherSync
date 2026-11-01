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
 * The OtherSync brand accent (the Other family brand sheet): bright teal on dark
 * surfaces, deep teal on light ones, and the brand's dark Ink for text and icons
 * that sit ON teal. White on deep teal is only 3.7:1, below the 4.5:1 the tests
 * ask of text, which is why the brand puts Ink on its teal buttons instead.
 *
 * The rest of each scheme is Material 3's own palette generated from the deep
 * teal (material-color-utilities, tonal spot), with the primary roles then
 * pointed at the brand colours, the same way the indigo schemes were made.
 */

/** OtherSync teal (#22D3C4): the accent on dark surfaces. */
val BrandTeal = Color(0xFF22D3C4)

/** Deep teal (#0D9488): the accent on light surfaces. */
val BrandDeepTeal = Color(0xFF0D9488)

/** The brand's Ink (#0A1118): text and icons on either teal. */
val BrandInk = Color(0xFF0A1118)

internal val LightColors = lightColorScheme(
    primary = BrandDeepTeal,
    onPrimary = BrandInk,
    primaryContainer = Color(0xFF73F8E7),
    onPrimaryContainer = Color(0xFF00201D),
    secondary = Color(0xFF4A635F),
    onSecondary = Color(0xFFFFFFFF),
    secondaryContainer = Color(0xFFCCE8E3),
    onSecondaryContainer = Color(0xFF05201C),
    tertiary = Color(0xFF46617A),
    onTertiary = Color(0xFFFFFFFF),
    tertiaryContainer = Color(0xFFCDE5FF),
    onTertiaryContainer = Color(0xFF001D32),
    error = Color(0xFFBA1A1A),
    onError = Color(0xFFFFFFFF),
    errorContainer = Color(0xFFFFDAD6),
    onErrorContainer = Color(0xFF410002),
    background = Color(0xFFFAFDFB),
    onBackground = Color(0xFF191C1B),
    surface = Color(0xFFFAFDFB),
    onSurface = Color(0xFF191C1B),
    surfaceVariant = Color(0xFFDAE5E2),
    onSurfaceVariant = Color(0xFF3F4947),
    outline = Color(0xFF6F7977),
    outlineVariant = Color(0xFFBEC9C6),
    inverseSurface = Color(0xFF2D3130),
    inverseOnSurface = Color(0xFFEFF1EF),
    // Snackbar actions sit on the dark inverse surface, so the bright teal.
    inversePrimary = BrandTeal,
    scrim = Color(0xFF000000),
)

internal val DarkColors = darkColorScheme(
    primary = BrandTeal,
    onPrimary = BrandInk,
    primaryContainer = Color(0xFF005049),
    onPrimaryContainer = Color(0xFF73F8E7),
    secondary = Color(0xFFB1CCC7),
    onSecondary = Color(0xFF1C3531),
    secondaryContainer = Color(0xFF324B48),
    onSecondaryContainer = Color(0xFFCCE8E3),
    tertiary = Color(0xFFAECAE6),
    onTertiary = Color(0xFF163349),
    tertiaryContainer = Color(0xFF2E4961),
    onTertiaryContainer = Color(0xFFCDE5FF),
    error = Color(0xFFFFB4AB),
    onError = Color(0xFF690005),
    errorContainer = Color(0xFF93000A),
    onErrorContainer = Color(0xFFFFB4AB),
    background = Color(0xFF191C1B),
    onBackground = Color(0xFFE0E3E1),
    surface = Color(0xFF191C1B),
    onSurface = Color(0xFFE0E3E1),
    surfaceVariant = Color(0xFF3F4947),
    onSurfaceVariant = Color(0xFFBEC9C6),
    outline = Color(0xFF899390),
    outlineVariant = Color(0xFF3F4947),
    inverseSurface = Color(0xFFE0E3E1),
    inverseOnSurface = Color(0xFF2D3130),
    // On the light inverse surface the deep teal is only 2.9:1, so this is
    // the generated tone-40 teal (5:1) rather than a brand hex.
    inversePrimary = Color(0xFF006A61),
    scrim = Color(0xFF000000),
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
