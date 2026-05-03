package com.example.taskflow.util

import android.content.Context
import android.net.ConnectivityManager
import android.net.NetworkCapabilities

/**
 * 简单的网络连通性检查工具。
 *
 * 离线策略:Windows 端与 Android 端在无网络时不允许新增或删除任务,
 * 仅允许查看和执行此前在有网状态下同步好的任务与提醒。
 */
object NetworkUtil {

    /** 当前设备是否有活跃的网络连接(Wi-Fi / 蜂窝 / 以太网)。 */
    fun isOnline(context: Context): Boolean {
        val cm = context.getSystemService(Context.CONNECTIVITY_SERVICE) as? ConnectivityManager
            ?: return false
        val net = cm.activeNetwork ?: return false
        val caps = cm.getNetworkCapabilities(net) ?: return false
        return caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
    }
}
