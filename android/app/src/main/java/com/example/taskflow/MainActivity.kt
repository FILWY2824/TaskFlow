package com.example.taskflow

import android.Manifest
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.BarChart
import androidx.compose.material.icons.filled.CalendarMonth
import androidx.compose.material.icons.filled.Home
import androidx.compose.material.icons.filled.Notifications
import androidx.compose.material.icons.filled.Person
import androidx.compose.material.icons.filled.Security
import androidx.compose.material.icons.filled.Send
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.Timer
import androidx.compose.material3.Button
import androidx.compose.material3.Icon
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.unit.dp
import androidx.navigation.NavGraph.Companion.findStartDestination
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import com.example.taskflow.ui.screens.CalendarScreen
import com.example.taskflow.ui.screens.LoginScreen
import com.example.taskflow.ui.screens.NotificationsScreen
import com.example.taskflow.ui.screens.PermissionCheckScreen
import com.example.taskflow.ui.screens.PomodoroScreen
import com.example.taskflow.ui.screens.ProductCard
import com.example.taskflow.ui.screens.ScreenIntro
import com.example.taskflow.ui.screens.SettingsScreen
import com.example.taskflow.ui.screens.StatsScreen
import com.example.taskflow.ui.screens.TasksScreen
import com.example.taskflow.ui.screens.TelegramBindScreen
import com.example.taskflow.ui.screens.TodoEditScreen
import com.example.taskflow.ui.theme.TaskFlowTheme

class MainActivity : ComponentActivity() {

    private val notifPermLauncher = registerForActivityResult(
        ActivityResultContracts.RequestPermission()
    ) { /* 权限自检页会继续提示 */ }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        maybeRequestNotifPerm()
        val container = (application as TaskFlowApp).container

        setContent {
            TaskFlowTheme {
                Surface(Modifier.fillMaxSize()) {
                    AppNav(container)
                }
            }
        }
    }

    private fun maybeRequestNotifPerm() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            val granted = checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) ==
                android.content.pm.PackageManager.PERMISSION_GRANTED
            if (!granted) notifPermLauncher.launch(Manifest.permission.POST_NOTIFICATIONS)
        }
    }
}

@Composable
private fun AppNav(container: AppContainer) {
    val nav = rememberNavController()
    val session by container.tokenManager.session.collectAsState()
    val start = if (session.isLoggedIn) Route.TASKS else Route.LOGIN
    val backStack by nav.currentBackStackEntryAsState()
    val current = backStack?.destination?.route
    val showBottomBar = session.isLoggedIn && current in BottomTab.items.map { it.route }

    LaunchedEffect(session.isLoggedIn) {
        if (!session.isLoggedIn) {
            val route = nav.currentBackStackEntry?.destination?.route
            if (route != null && route != Route.LOGIN) {
                nav.navigate(Route.LOGIN) { popUpTo(0) { inclusive = true } }
            }
        }
    }

    Scaffold(
        bottomBar = {
            if (showBottomBar) {
                NavigationBar {
                    BottomTab.items.forEach { tab ->
                        NavigationBarItem(
                            selected = current == tab.route,
                            onClick = {
                                nav.navigate(tab.route) {
                                    popUpTo(nav.graph.findStartDestination().id) { saveState = true }
                                    launchSingleTop = true
                                    restoreState = true
                                }
                            },
                            icon = { Icon(tab.icon, contentDescription = tab.label) },
                            label = { Text(tab.label) },
                        )
                    }
                }
            }
        },
    ) { innerPadding ->
        NavHost(
            navController = nav,
            startDestination = start,
            modifier = Modifier.padding(if (showBottomBar) innerPadding else PaddingValues(0.dp)),
        ) {
            composable(Route.LOGIN) {
                LoginScreen(
                    container = container,
                    onSuccess = {
                        nav.navigate(Route.TASKS) {
                            popUpTo(Route.LOGIN) { inclusive = true }
                        }
                    },
                )
            }

            composable(Route.TASKS) {
                TasksScreen(
                    container = container,
                    onOpenTodo = { id ->
                        nav.navigate(if (id == null) "${Route.TODO_EDIT_PREFIX}/0" else "${Route.TODO_EDIT_PREFIX}/$id")
                    },
                    onOpenSettings = { nav.navigate(Route.MORE) },
                    onOpenNotifications = { nav.navigate(Route.NOTIFS) },
                    onOpenStats = { nav.navigate(Route.STATS) },
                    onOpenPomodoro = { nav.navigate(Route.POMODORO) },
                    onOpenTelegram = { nav.navigate(Route.TELEGRAM) },
                    onOpenPermissions = { nav.navigate(Route.PERMS) },
                    onOpenCalendar = { nav.navigate(Route.CALENDAR) },
                )
            }

            composable(
                "${Route.TODO_EDIT_PREFIX}/{id}",
                arguments = listOf(navArgument("id") { type = NavType.LongType }),
            ) { backStackEntry ->
                val id = backStackEntry.arguments?.getLong("id") ?: 0L
                TodoEditScreen(
                    container = container,
                    todoId = id.takeIf { it > 0 },
                    onBack = { nav.popBackStack() },
                )
            }

            composable(Route.CALENDAR) {
                CalendarScreen(
                    container = container,
                    onBack = { nav.navigate(Route.TASKS) },
                    onOpenTodo = { id -> nav.navigate("${Route.TODO_EDIT_PREFIX}/$id") },
                )
            }
            composable(Route.POMODORO) { PomodoroScreen(container, onBack = { nav.navigate(Route.TASKS) }) }
            composable(Route.STATS) { StatsScreen(container, onBack = { nav.navigate(Route.TASKS) }) }
            composable(Route.MORE) {
                MoreScreen(
                    onOpenSettings = { nav.navigate(Route.SETTINGS) },
                    onOpenNotifications = { nav.navigate(Route.NOTIFS) },
                    onOpenTelegram = { nav.navigate(Route.TELEGRAM) },
                    onOpenPermissions = { nav.navigate(Route.PERMS) },
                )
            }

            composable(Route.NOTIFS) { NotificationsScreen(container, onBack = { nav.popBackStack() }) }
            composable(Route.TELEGRAM) { TelegramBindScreen(container, onBack = { nav.popBackStack() }) }
            composable(Route.PERMS) { PermissionCheckScreen(onBack = { nav.popBackStack() }) }
            composable(Route.SETTINGS) {
                SettingsScreen(
                    container = container,
                    onBack = { nav.popBackStack() },
                    onLoggedOut = {
                        nav.navigate(Route.LOGIN) {
                            popUpTo(0) { inclusive = true }
                        }
                    },
                )
            }
        }
    }
}

@Composable
private fun MoreScreen(
    onOpenSettings: () -> Unit,
    onOpenNotifications: () -> Unit,
    onOpenTelegram: () -> Unit,
    onOpenPermissions: () -> Unit,
) {
    Column(
        Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        ProductCard(tonal = true) {
            ScreenIntro(
                title = "我的",
                subtitle = "管理账号、提醒权限、Telegram 推送和版本更新。",
            )
        }
        MoreAction("通知中心", "查看强提醒和已读状态", Icons.Default.Notifications, onOpenNotifications)
        MoreAction("Telegram 推送", "绑定账号，把重要提醒同步到 Telegram", Icons.Default.Send, onOpenTelegram)
        MoreAction("提醒权限", "检查锁屏全屏、精确闹钟和通知权限", Icons.Default.Security, onOpenPermissions)
        MoreAction("设置", "账号、时区、Android 提醒行为和版本更新", Icons.Default.Settings, onOpenSettings)
    }
}

@Composable
private fun MoreAction(
    title: String,
    subtitle: String,
    icon: ImageVector,
    onClick: () -> Unit,
) {
    ProductCard {
        ListItem(
            headlineContent = { Text(title) },
            supportingContent = { Text(subtitle) },
            leadingContent = { Icon(icon, contentDescription = null, tint = MaterialTheme.colorScheme.primary) },
            trailingContent = { Button(onClick = onClick) { Text("打开") } },
        )
    }
}

private data class BottomTab(
    val route: String,
    val label: String,
    val icon: ImageVector,
) {
    companion object {
        val items = listOf(
            BottomTab(Route.TASKS, "工作台", Icons.Default.Home),
            BottomTab(Route.CALENDAR, "日程", Icons.Default.CalendarMonth),
            BottomTab(Route.POMODORO, "专注", Icons.Default.Timer),
            BottomTab(Route.STATS, "统计", Icons.Default.BarChart),
            BottomTab(Route.MORE, "我的", Icons.Default.Person),
        )
    }
}

private object Route {
    const val LOGIN = "login"
    const val TASKS = "tasks"
    const val TODO_EDIT_PREFIX = "todo_edit"
    const val NOTIFS = "notifs"
    const val STATS = "stats"
    const val POMODORO = "pomodoro"
    const val TELEGRAM = "telegram"
    const val PERMS = "perms"
    const val SETTINGS = "settings"
    const val CALENDAR = "calendar"
    const val MORE = "more"
}
