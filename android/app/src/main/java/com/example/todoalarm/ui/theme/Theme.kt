package com.example.todoalarm.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

// 与 Web 端 (web/src/style.css) 主色统一:#1F6FEB 蓝。
private val LightColors = lightColorScheme(
    primary = Color(0xFF1F6FEB),
    onPrimary = Color.White,
    primaryContainer = Color(0xFFDBE9FE),
    onPrimaryContainer = Color(0xFF062A6E),
    secondary = Color(0xFF4B5563),
    background = Color(0xFFF7F8FA),
    surface = Color.White,
    error = Color(0xFFEF4444),
)

private val DarkColors = darkColorScheme(
    primary = Color(0xFF58A6FF),
    onPrimary = Color.White,
    primaryContainer = Color(0xFF1F3A5F),
    onPrimaryContainer = Color(0xFFDBE9FE),
    secondary = Color(0xFFB6BEC7),
    background = Color(0xFF0D1117),
    surface = Color(0xFF161B22),
    error = Color(0xFFEF4444),
)

@Composable
fun TaskFlowTheme(
    useDarkTheme: Boolean = isSystemInDarkTheme(),
    content: @Composable () -> Unit
) {
    MaterialTheme(
        colorScheme = if (useDarkTheme) DarkColors else LightColors,
        content = content,
    )
}
