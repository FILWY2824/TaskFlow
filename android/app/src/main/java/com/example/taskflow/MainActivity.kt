package com.example.taskflow

import android.Manifest
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.Surface
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import com.example.taskflow.ui.screens.CalendarScreen
import com.example.taskflow.ui.screens.LoginScreen
import com.example.taskflow.ui.screens.NotificationsScreen
import com.example.taskflow.ui.screens.PermissionCheckScreen
import com.example.taskflow.ui.screens.PomodoroScreen
import com.example.taskflow.ui.screens.SettingsScreen
import com.example.taskflow.ui.screens.StatsScreen
import com.example.taskflow.ui.screens.TasksScreen
import com.example.taskflow.ui.screens.TelegramBindScreen
import com.example.taskflow.ui.screens.TodoEditScreen
import com.example.taskflow.ui.theme.TaskFlowTheme

/**
 * 主入口。launchMode = singleTask(在 manifest 里),保证从通知 / 深链拉起时复用栈。
 *
 * 启动时:
 *   1. 检查 POST_NOTIFICATIONS(Android 13+),没有就请求一次。
 *   2. 根据 TokenManager.session.isLoggedIn 决定起始路由是 login 还是 today。
 *   3. 路由表见 routes 字符串常量。
 */
class MainActivity : ComponentActivity() {

    private val notifPermLauncher = registerForActivityResult(
        ActivityResultContracts.RequestPermission()
    ) { /* 不强制成功;PermissionCheckScreen 会持续提示 */ }

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

@androidx.compose.runtime.Composable
private fun AppNav(container: AppContainer) {
    val nav = rememberNavController()
    val session by container.tokenManager.session.collectAsState()

    val start = if (session.isLoggedIn) Route.TASKS else Route.LOGIN

    // 当后台 401 把 token 清空(AuthInterceptor 处理),自动跳回登录页。
    // 唯一例外:用户已经在 login 页面,就不要循环跳了。
    androidx.compose.runtime.LaunchedEffect(session.isLoggedIn) {
        if (!session.isLoggedIn) {
            val current = nav.currentBackStackEntry?.destination?.route
            if (current != null && current != Route.LOGIN) {
                nav.navigate(Route.LOGIN) { popUpTo(0) { inclusive = true } }
            }
        }
    }

    NavHost(navController = nav, startDestination = start) {

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
                onOpenSettings = { nav.navigate(Route.SETTINGS) },
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
        ) { backStack ->
            val id = backStack.arguments?.getLong("id") ?: 0L
            TodoEditScreen(container = container, todoId = id.takeIf { it > 0 }, onBack = { nav.popBackStack() })
        }

        composable(Route.NOTIFS) { NotificationsScreen(container, onBack = { nav.popBackStack() }) }
        composable(Route.STATS) { StatsScreen(container, onBack = { nav.popBackStack() }) }
        composable(Route.POMODORO) { PomodoroScreen(container, onBack = { nav.popBackStack() }) }
        composable(Route.TELEGRAM) { TelegramBindScreen(container, onBack = { nav.popBackStack() }) }
        composable(Route.PERMS) { PermissionCheckScreen(onBack = { nav.popBackStack() }) }
        composable(Route.CALENDAR) {
            CalendarScreen(
                container = container,
                onBack = { nav.popBackStack() },
                onOpenTodo = { id -> nav.navigate("${Route.TODO_EDIT_PREFIX}/$id") },
            )
        }

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
}
