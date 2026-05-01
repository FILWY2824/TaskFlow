package com.example.todoalarm

import android.app.Application
import androidx.room.Room
import com.example.todoalarm.alarm.AlarmForegroundService
import com.example.todoalarm.alarm.AlarmScheduler
import com.example.todoalarm.data.auth.TokenManager
import com.example.todoalarm.data.local.AppDatabase
import com.example.todoalarm.data.remote.ApiClient
import com.example.todoalarm.data.repository.AuthRepository
import com.example.todoalarm.data.repository.ListRepository
import com.example.todoalarm.data.repository.NotificationRepository
import com.example.todoalarm.data.repository.PomodoroRepository
import com.example.todoalarm.data.repository.ReminderRepository
import com.example.todoalarm.data.repository.StatsRepository
import com.example.todoalarm.data.repository.SubtaskRepository
import com.example.todoalarm.data.repository.TelegramRepository
import com.example.todoalarm.data.repository.TodoRepository
import com.example.todoalarm.sync.SyncWorker

/**
 * Application class —— 启动时建立全部单例。
 *
 * 不用 Hilt:对一个体量这么小的 app(<20 个注入点),手写 container 比注解处理器轻得多。
 * Application.container 是个简单的全局服务定位器,Activity / Receiver / Service / Worker 都可以从这里拿。
 */
class TaskFlowApp : Application() {

    lateinit var container: AppContainer
        private set

    override fun onCreate() {
        super.onCreate()
        container = AppContainer(this)

        // 启动时:确保通知 channel 存在
        AlarmForegroundService.ensureChannel(this)

        // 周期同步(WorkManager 会去重)
        SyncWorker.schedulePeriodic(this)
    }
}

class AppContainer(private val app: TaskFlowApp) {

    val tokenManager: TokenManager by lazy { TokenManager(app) }

    val db: AppDatabase by lazy {
        Room.databaseBuilder(
            app, AppDatabase::class.java, "todoalarm.db",
        )
        .fallbackToDestructiveMigration() // MVP:schema 升级时直接重建本地缓存,服务端是 source of truth
        .build()
    }

    val apiClient: ApiClient by lazy { ApiClient(tokenManager) }

    val alarmScheduler: AlarmScheduler by lazy { AlarmScheduler(app, db.reminderDao()) }

    // Repositories
    val authRepository: AuthRepository by lazy { AuthRepository(apiClient, tokenManager, db) }
    val todoRepository: TodoRepository by lazy { TodoRepository(apiClient, db, tokenManager) }
    val listRepository: ListRepository by lazy { ListRepository(apiClient, db, tokenManager) }
    val subtaskRepository: SubtaskRepository by lazy { SubtaskRepository(apiClient, db) }
    val reminderRepository: ReminderRepository by lazy { ReminderRepository(apiClient, db, tokenManager, alarmScheduler) }
    val notificationRepository: NotificationRepository by lazy { NotificationRepository(apiClient) }
    val telegramRepository: TelegramRepository by lazy { TelegramRepository(apiClient) }
    val statsRepository: StatsRepository by lazy { StatsRepository(apiClient) }
    val pomodoroRepository: PomodoroRepository by lazy { PomodoroRepository(apiClient) }
}
