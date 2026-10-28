/*
 * BaseDirName.kt — which folder in shared storage this install syncs into.
 *
 * The app was called Nimbo, and every install from then syncs into
 * <external storage>/Nimbo. The app re-points the core at the default folder
 * on every engine start, so simply changing the default would quietly switch
 * an existing install to a new, empty OtherSync folder (and new folders would
 * land there, away from the user's files). So the name is decided once per
 * install and then pinned: an install that was already in use keeps Nimbo,
 * a new one gets OtherSync. The user's files are never moved.
 */
package dev.otherworld.nimbo.core

import android.content.Context
import android.os.Environment
import android.util.Log
import androidx.core.content.edit
import java.io.File

internal object BaseDirName {

    /** New installs. */
    const val CURRENT = "OtherSync"

    /** Every install from before the rename. */
    const val LEGACY = "Nimbo"

    private const val TAG = "NimboBaseDir"
    private const val PREFS = "base_dir"
    private const val KEY_NAME = "name"

    /**
     * The folder name for this install. [stored] wins once it is set. If not,
     * an install already in use (signed in, or the old folder is on the
     * device) keeps [LEGACY]; anything else is a new install and gets
     * [CURRENT]. A stored value that is blank or unknown counts as unset.
     */
    fun choose(stored: String?, hasAccount: Boolean, legacyFolderExists: Boolean): String = when {
        stored == CURRENT || stored == LEGACY -> stored
        hasAccount || legacyFolderExists -> LEGACY
        else -> CURRENT
    }

    /**
     * Reads the pinned name, deciding and pinning it on the first run. Only
     * pins when [hasAccount] could actually be answered ([canPin]): if the core
     * failed to start this process, it returns the best guess and decides
     * again next time. Never throws.
     */
    fun resolve(context: Context, hasAccount: Boolean, canPin: Boolean): String {
        val prefs = runCatching {
            context.applicationContext.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
        }.getOrNull()
        val stored = runCatching { prefs?.getString(KEY_NAME, null) }.getOrNull()
        val legacyExists = runCatching {
            File(Environment.getExternalStorageDirectory(), LEGACY).isDirectory
        }.getOrDefault(false)
        val name = choose(stored, hasAccount, legacyExists)
        if (name != stored && canPin && prefs != null) {
            runCatching { prefs.edit(commit = true) { putString(KEY_NAME, name) } }
                .onSuccess { Log.i(TAG, "sync folder name pinned to $name") }
                .onFailure { Log.w(TAG, "could not pin the sync folder name", it) }
        }
        return name
    }
}
