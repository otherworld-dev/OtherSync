/*
 * SettingsScreen.kt — the app's own preferences, as opposed to the account's.
 *
 * Appearance, the way in to supporting Nimbo, and signing out. Everything else Nimbo does is decided by the sync
 * engine or by the server, and inventing settings for their own sake gives
 * people more ways to break something than to fix it.
 */
package dev.otherworld.nimbo.ui.screens

import androidx.compose.foundation.Image
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Circle
import androidx.compose.material.icons.filled.Favorite
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import dev.otherworld.nimbo.core.NcAccount
import androidx.compose.runtime.remember
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.runtime.getValue
import androidx.compose.material3.TextButton
import androidx.compose.material3.AlertDialog
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import androidx.compose.ui.graphics.Color
import dev.otherworld.nimbo.R
import dev.otherworld.nimbo.supporter.AccentChoice
import dev.otherworld.nimbo.supporter.AppIcon
import dev.otherworld.nimbo.supporter.SupporterTier
import dev.otherworld.nimbo.supporter.SupporterUi
import dev.otherworld.nimbo.supporter.supporterAccentArgb
import dev.otherworld.nimbo.ui.theme.AccentSource
import dev.otherworld.nimbo.ui.theme.AppearancePreference
import dev.otherworld.nimbo.ui.theme.fitAccentArgb
import dev.otherworld.nimbo.ui.theme.parseThemeColor
import androidx.compose.foundation.background
import androidx.compose.material3.OutlinedTextField
import androidx.compose.ui.graphics.luminance
import androidx.compose.ui.graphics.toArgb

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(
    appearance: AppearancePreference,
    serverAppearance: String,
    themeColor: String,
    accentSource: AccentSource,
    supporter: SupporterUi,
    onAppearanceChange: (AppearancePreference) -> Unit,
    onAccentSourceChange: (AccentSource) -> Unit,
    onCustomAccent: (Long) -> Unit,
    onIconChange: (AppIcon) -> Unit,
    onAccentChange: (AccentChoice) -> Unit,
    onOpenSupport: () -> Unit,
    account: NcAccount?,
    onSignOut: () -> Unit,
    onBack: () -> Unit,
) {
    var confirmSignOut by remember { mutableStateOf(false) }

    Scaffold(
        modifier = Modifier.fillMaxSize(),
        topBar = { NimboTopBar(title = "Settings", onBack = onBack) },
    ) { inner ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(inner)
                .verticalScroll(rememberScrollState()),
        ) {
            SupportRow(tier = supporter.tier, onClick = onOpenSupport)
            HorizontalDivider(modifier = Modifier.padding(vertical = 8.dp))
            SectionHeader("Appearance")
            Column(modifier = Modifier.selectableGroup()) {
                AppearancePreference.entries.forEach { option ->
                    AppearanceRow(
                        option = option,
                        selected = option == appearance,
                        subtitle = subtitleFor(option, serverAppearance),
                        onSelect = { onAppearanceChange(option) },
                    )
                }
            }

            HorizontalDivider(modifier = Modifier.padding(vertical = 8.dp))
            SectionHeader("Accent colour")
            Column(modifier = Modifier.selectableGroup()) {
                AccentSource.entries.forEach { source ->
                    AccentSourceRow(
                        source = source,
                        themeColor = themeColor,
                        selected = source == accentSource,
                        onSelect = { onAccentSourceChange(source) },
                    )
                }
            }
            if (supporter.effectiveAccent != AccentChoice.FOLLOW_NEXTCLOUD) {
                Text(
                    "Your supporter colour below is in use. Pick Default there to go back to this setting.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp),
                )
            }
            Spacer(Modifier.height(8.dp))
            if (supporter.perks.accents) {
                var editingCustom by remember { mutableStateOf(false) }
                val dark = MaterialTheme.colorScheme.surface.luminance() < 0.5f
                Text(
                    "Supporter colours",
                    style = MaterialTheme.typography.bodyMedium,
                    modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp),
                )
                Column(modifier = Modifier.selectableGroup()) {
                    AccentChoice.entries.filter { it != AccentChoice.CUSTOM }.forEach { choice ->
                        AccentChoiceRow(
                            label = choice.label,
                            swatch = choice.argbFor(dark),
                            selected = choice == supporter.accent,
                            onSelect = { onAccentChange(choice) },
                        )
                    }
                    AccentChoiceRow(
                        label = if (supporter.customAccent == null) "Custom (pick a colour)" else "Custom",
                        swatch = supporterAccentArgb(AccentChoice.CUSTOM, supporter.customAccent, dark),
                        selected = supporter.accent == AccentChoice.CUSTOM,
                        onSelect = {
                            if (supporter.customAccent == null) editingCustom = true
                            else onAccentChange(AccentChoice.CUSTOM)
                        },
                        trailing = { TextButton(onClick = { editingCustom = true }) { Text("Change") } },
                    )
                }
                if (editingCustom) {
                    CustomAccentDialog(
                        initial = supporter.customAccent,
                        onDismiss = { editingCustom = false },
                        onSave = { argb ->
                            editingCustom = false
                            onCustomAccent(argb)
                        },
                    )
                }
            } else {
                LockedPerkRow("Accent colours", SupporterTier.PATRON, onOpenSupport)
            }

            HorizontalDivider(modifier = Modifier.padding(vertical = 8.dp))
            SectionHeader("App icon")
            if (supporter.perks.icons) {
                Column(modifier = Modifier.selectableGroup()) {
                    AppIcon.entries.forEach { icon ->
                        IconRow(icon = icon, selected = icon == supporter.icon, onSelect = { onIconChange(icon) })
                    }
                }
                Text(
                    "Your new icon appears after you leave OtherSync. Your home-screen shortcut may need re-adding.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
                )
            } else {
                LockedPerkRow("Alternative app icons", SupporterTier.BACKER, onOpenSupport)
            }

            HorizontalDivider(modifier = Modifier.padding(vertical = 8.dp))
            SectionHeader("Account")
            if (account != null) {
                Text(
                    accountLabel(account),
                    style = MaterialTheme.typography.bodyMedium,
                    modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp),
                )
            }
            TextButton(
                onClick = { confirmSignOut = true },
                modifier = Modifier.padding(horizontal = 8.dp),
            ) {
                Text("Sign out", color = MaterialTheme.colorScheme.error)
            }
            Spacer(Modifier.height(16.dp))
        }
    }

    if (confirmSignOut) {
        AlertDialog(
            onDismissRequest = { confirmSignOut = false },
            title = { Text("Sign out?") },
            text = {
                Text(
                    "OtherSync stops syncing on this phone until you sign in again. " +
                        "Your files stay on the server.",
                )
            },
            confirmButton = {
                TextButton(onClick = { confirmSignOut = false; onSignOut() }) {
                    Text("Sign out")
                }
            },
            dismissButton = {
                TextButton(onClick = { confirmSignOut = false }) { Text("Cancel") }
            },
        )
    }
}

/** "login@host", the way the account is named everywhere else. */
private fun accountLabel(account: NcAccount): String {
    val host = account.serverURL.substringAfter("://").trimEnd('/')
    return if (account.loginName.isNotEmpty()) "${account.loginName}@$host" else host
}

/**
 * What each option will actually do right now — the useful half of the choice.
 * "Follow Nextcloud" in particular does something different depending on a
 * setting the user made somewhere else entirely, so it says which.
 */
private fun subtitleFor(option: AppearancePreference, serverAppearance: String): String =
    when (option) {
        AppearancePreference.FOLLOW_NEXTCLOUD -> when (serverAppearance.trim().lowercase()) {
            "dark" -> "Your Nextcloud is set to dark"
            "light" -> "Your Nextcloud is set to light"
            "" -> "Your Nextcloud hasn't said yet — the phone decides"
            else -> "Your Nextcloud follows its own system, so the phone decides"
        }
        AppearancePreference.FOLLOW_SYSTEM -> "Matches your phone's dark mode"
        AppearancePreference.ALWAYS_DARK -> "Dark, whatever the phone or server says"
        AppearancePreference.ALWAYS_LIGHT -> "Light, whatever the phone or server says"
    }

@Composable
private fun AppearanceRow(
    option: AppearancePreference,
    selected: Boolean,
    subtitle: String,
    onSelect: () -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .selectable(selected = selected, role = Role.RadioButton, onClick = onSelect)
            .padding(horizontal = 12.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        RadioButton(selected = selected, onClick = null)
        Spacer(Modifier.width(8.dp))
        Column {
            Text(option.label, style = MaterialTheme.typography.bodyLarge)
            Text(
                text = subtitle,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

/**
 * One choice of the free Accent colour setting. The server option shows the
 * colour the server actually gives, because a colour arriving from somewhere
 * else is otherwise a mystery.
 */
@Composable
private fun AccentSourceRow(source: AccentSource, themeColor: String, selected: Boolean, onSelect: () -> Unit) {
    val server = parseThemeColor(themeColor)
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .selectable(selected = selected, role = Role.RadioButton, onClick = onSelect)
            .padding(horizontal = 12.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        RadioButton(selected = selected, onClick = null)
        Spacer(Modifier.width(8.dp))
        Column(modifier = Modifier.weight(1f)) {
            Text(source.label, style = MaterialTheme.typography.bodyLarge)
            Text(
                text = when (source) {
                    AccentSource.BRAND -> "Valley green"
                    AccentSource.SERVER -> if (server != null) themeColor.trim().uppercase()
                    else "Your server has not given one, so OtherSync's colour is used"
                },
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        if (source == AccentSource.SERVER && server != null) {
            Icon(Icons.Filled.Circle, contentDescription = null, tint = server, modifier = Modifier.size(24.dp))
        }
    }
}

/** First in Settings: the way in to supporting, and where the badge lives. */
@Composable
private fun SupportRow(tier: SupporterTier, onClick: () -> Unit) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onClick)
            .padding(horizontal = 16.dp, vertical = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            Icons.Filled.Favorite,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.primary,
            modifier = Modifier.size(24.dp),
        )
        Spacer(Modifier.width(14.dp))
        Column(modifier = Modifier.weight(1f)) {
            Text("Support OtherSync", style = MaterialTheme.typography.bodyLarge)
            Text(
                if (tier == SupporterTier.NONE) "OtherSync is free. Support it if it's useful to you"
                else "Thank you for supporting OtherSync",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        SupporterBadge(tier)
    }
}

@Composable
private fun IconRow(icon: AppIcon, selected: Boolean, onSelect: () -> Unit) {
    val (background, foreground) = when (icon) {
        AppIcon.DEFAULT -> R.drawable.ic_launcher_background to R.drawable.ic_launcher_foreground
        AppIcon.FOREST -> R.drawable.ic_launcher_forest_background to R.drawable.ic_launcher_forest_foreground
        AppIcon.EMBER -> R.drawable.ic_launcher_ember_background to R.drawable.ic_launcher_ember_foreground
        AppIcon.SLATE -> R.drawable.ic_launcher_slate_background to R.drawable.ic_launcher_slate_foreground
    }
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .selectable(selected = selected, role = Role.RadioButton, onClick = onSelect)
            .padding(horizontal = 12.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        RadioButton(selected = selected, onClick = null)
        Spacer(Modifier.width(8.dp))
        // The adaptive icon's two layers, stacked: an <adaptive-icon> itself
        // can't be loaded by painterResource.
        Box(modifier = Modifier.size(40.dp).clip(CircleShape)) {
            Image(painterResource(background), contentDescription = null, modifier = Modifier.fillMaxSize())
            Image(painterResource(foreground), contentDescription = null, modifier = Modifier.fillMaxSize())
        }
        Spacer(Modifier.width(12.dp))
        Text(icon.label, style = MaterialTheme.typography.bodyLarge)
    }
}

@Composable
private fun AccentChoiceRow(
    label: String,
    swatch: Long?,
    selected: Boolean,
    onSelect: () -> Unit,
    trailing: (@Composable () -> Unit)? = null,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .selectable(selected = selected, role = Role.RadioButton, onClick = onSelect)
            .padding(horizontal = 12.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        RadioButton(selected = selected, onClick = null)
        Spacer(Modifier.width(8.dp))
        Icon(
            imageVector = Icons.Filled.Circle,
            contentDescription = null,
            tint = swatch?.let { Color(it) } ?: MaterialTheme.colorScheme.outline,
            modifier = Modifier.size(24.dp),
        )
        Spacer(Modifier.width(12.dp))
        Text(label, style = MaterialTheme.typography.bodyLarge, modifier = Modifier.weight(1f))
        trailing?.invoke()
    }
}

/**
 * Pick a custom accent by colour code. Shows the colour as it will actually be
 * used on the light and dark themes: fitAccentArgb nudges it lighter or
 * darker where needed so it always stands out and keeps text readable.
 */
@Composable
private fun CustomAccentDialog(initial: Long?, onDismiss: () -> Unit, onSave: (Long) -> Unit) {
    var text by remember {
        mutableStateOf(initial?.let { "#%06X".format(it and 0xFFFFFF) } ?: "#")
    }
    val argb = parseThemeColor(text)?.toArgb()?.toLong()?.and(0xFFFFFFFFL)
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Custom colour") },
        text = {
            Column {
                OutlinedTextField(
                    value = text,
                    onValueChange = { text = it.take(7) },
                    label = { Text("Colour code, e.g. #3A7BD5") },
                    singleLine = true,
                    isError = text.length > 1 && argb == null,
                )
                Spacer(Modifier.height(12.dp))
                if (argb != null) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        PreviewChip("Light", fitAccentArgb(argb, dark = false), Color(0xFFF4F0E8))
                        Spacer(Modifier.width(16.dp))
                        PreviewChip("Dark", fitAccentArgb(argb, dark = true), Color(0xFF23262B))
                    }
                    Text(
                        "Adjusted where needed so it stays readable.",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(top = 8.dp),
                    )
                }
            }
        },
        confirmButton = {
            TextButton(onClick = { argb?.let(onSave) }, enabled = argb != null) { Text("Use colour") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

/** The accent as it will look, on the theme's own background. */
@Composable
private fun PreviewChip(label: String, argb: Long, background: Color) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        Box(
            modifier = Modifier
                .size(36.dp)
                .clip(CircleShape)
                .background(background),
            contentAlignment = Alignment.Center,
        ) {
            Icon(Icons.Filled.Circle, contentDescription = null, tint = Color(argb), modifier = Modifier.size(24.dp))
        }
        Spacer(Modifier.width(6.dp))
        Text(label, style = MaterialTheme.typography.bodySmall)
    }
}
