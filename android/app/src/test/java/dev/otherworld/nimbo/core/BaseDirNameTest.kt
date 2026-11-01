package dev.otherworld.nimbo.core

import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * The sync folder name after the rename: an install already in use keeps
 * "Nimbo", a new one gets "OtherSync", and once decided it never changes.
 */
class BaseDirNameTest {

    @Test
    fun `a new install gets OtherSync`() {
        assertEquals("OtherSync", BaseDirName.choose(stored = null, hasAccount = false, legacyFolderExists = false))
    }

    @Test
    fun `an install with an account keeps Nimbo`() {
        assertEquals("Nimbo", BaseDirName.choose(stored = null, hasAccount = true, legacyFolderExists = false))
    }

    @Test
    fun `an install whose Nimbo folder is on the device keeps Nimbo`() {
        // Signed out, but the folder (and the files in it) are still there.
        assertEquals("Nimbo", BaseDirName.choose(stored = null, hasAccount = false, legacyFolderExists = true))
    }

    @Test
    fun `a pinned name wins over everything`() {
        // A new install pinned OtherSync, then signed in: it stays OtherSync.
        assertEquals("OtherSync", BaseDirName.choose(stored = "OtherSync", hasAccount = true, legacyFolderExists = true))
        // An upgraded install pinned Nimbo, then signed out: it stays Nimbo.
        assertEquals("Nimbo", BaseDirName.choose(stored = "Nimbo", hasAccount = false, legacyFolderExists = false))
    }

    @Test
    fun `a blank or unknown stored value is decided again`() {
        assertEquals("Nimbo", BaseDirName.choose(stored = "", hasAccount = true, legacyFolderExists = false))
        assertEquals("OtherSync", BaseDirName.choose(stored = "Elsewhere", hasAccount = false, legacyFolderExists = false))
    }

    @Test
    fun `the default for new installs is OtherSync`() {
        assertEquals("OtherSync", NimboCore.DEFAULT_BASE_DIR_NAME)
        assertEquals("Nimbo", BaseDirName.LEGACY)
    }
}
