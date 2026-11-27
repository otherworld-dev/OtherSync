/*
 * SupporterPrefs.kt — the supporter state that lives on this phone: the
 * cached status and the user's choices. Not secret (keys live in
 * KeystoreSecretStore), so plain SharedPreferences. Reads never throw.
 */
package dev.otherworld.nimbo.supporter

import android.content.Context
import android.content.SharedPreferences
import android.util.Log
import androidx.core.content.edit

class SupporterPrefs(context: Context) : StatusCache {

    private val prefs: SharedPreferences =
        context.applicationContext.getSharedPreferences(PREFS, Context.MODE_PRIVATE)

    override fun read(): SupporterStatus =
        SupporterStatus.decode(runCatching { prefs.getString(KEY_STATUS, null) }.getOrNull())

    override fun write(status: SupporterStatus) = save { putString(KEY_STATUS, status.encode()) }

    var icon: AppIcon
        get() = AppIcon.fromStored(string(KEY_ICON))
        set(value) = save { putString(KEY_ICON, value.name) }

    var accent: AccentChoice
        get() = AccentChoice.fromStored(string(KEY_ACCENT))
        set(value) = save { putString(KEY_ACCENT, value.name) }

    /** The supporter's custom accent (0xAARRGGBB), null until one is saved. */
    var customAccent: Long?
        get() = runCatching { if (prefs.contains(KEY_CUSTOM_ACCENT)) prefs.getLong(KEY_CUSTOM_ACCENT, 0L) else null }.getOrNull()
        set(value) = save { if (value == null) remove(KEY_CUSTOM_ACCENT) else putLong(KEY_CUSTOM_ACCENT, value) }

    var firstSeenAt: Long
        get() = runCatching { prefs.getLong(KEY_FIRST_SEEN, 0L) }.getOrDefault(0L)
        set(value) = save { putLong(KEY_FIRST_SEEN, value) }

    var nudgeRetired: Boolean
        get() = runCatching { prefs.getBoolean(KEY_NUDGE_RETIRED, false) }.getOrDefault(false)
        set(value) = save { putBoolean(KEY_NUDGE_RETIRED, value) }

    fun choices() = SupporterChoices(
        icon = icon,
        accent = accent,
        customAccent = customAccent,
        firstSeenAt = firstSeenAt,
        nudgeRetired = nudgeRetired,
    )

    private fun string(key: String): String? = runCatching { prefs.getString(key, null) }.getOrNull()

    private fun save(block: SharedPreferences.Editor.() -> Unit) {
        runCatching { prefs.edit(action = block) }
            .onFailure { Log.w(TAG, "could not save a supporter preference", it) }
    }

    private companion object {
        const val TAG = "NimboSupporter"
        const val PREFS = "supporter_prefs"
        const val KEY_STATUS = "status"
        const val KEY_ICON = "icon"
        const val KEY_ACCENT = "accent"
        const val KEY_CUSTOM_ACCENT = "custom_accent"
        const val KEY_FIRST_SEEN = "first_seen_at"
        const val KEY_NUDGE_RETIRED = "nudge_retired"
    }
}
