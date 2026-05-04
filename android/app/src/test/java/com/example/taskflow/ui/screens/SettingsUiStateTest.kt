package com.example.taskflow.ui.screens

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class SettingsUiStateTest {
    @Test
    fun updateCheckResultIsPresentedAsDialogState() {
        val state = SettingsUiState().withUpdateDialog(
            hasNew = true,
            version = "1.1.0",
            url = "http://127.0.0.1:8080/downloads/android/TaskFlow-debug.apk",
            notes = "修复 Android 提醒",
        )

        assertEquals("1.1.0", state.updateDialog?.version)
        assertEquals("修复 Android 提醒", state.updateDialog?.notes)
        assertEquals(true, state.updateDialog?.hasNew)
        assertNull(state.updateHasNew)
        assertNull(state.updateVersion)
        assertNull(state.updateNotes)
    }

    @Test
    fun dismissedUpdateDialogClearsDialogState() {
        val state = SettingsUiState().withUpdateDialog(
            hasNew = false,
            version = "1.4.0",
            url = null,
            notes = "当前已是最新版本",
        )

        assertNull(state.dismissUpdateDialog().updateDialog)
    }

    @Test
    fun timezoneSyncPromptIsVisibleOnlyWhenSystemTimezoneDiffers() {
        val same = SettingsUiState(timezone = "Asia/Shanghai", systemTimezone = "Asia/Shanghai")
        val different = SettingsUiState(timezone = "Asia/Shanghai", systemTimezone = "America/New_York")

        assertEquals(false, same.shouldSuggestSystemTimezone)
        assertEquals(true, different.shouldSuggestSystemTimezone)
    }

    @Test
    fun timezoneOptionsStartFromShanghaiAndAllowManualSelection() {
        assertEquals("Asia/Shanghai", DEFAULT_TIMEZONE)
        assertEquals("中国上海 (UTC+8)", timezoneLabel("Asia/Shanghai"))
        assertEquals("美国洛杉矶 (UTC-8/-7)", timezoneLabel("America/Los_Angeles"))
    }
}
