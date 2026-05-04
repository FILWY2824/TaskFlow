package com.example.taskflow.ui.screens

const val DEFAULT_TIMEZONE = "Asia/Shanghai"

data class TimezoneOption(
    val value: String,
    val label: String,
)

data class TimezoneGroup(
    val label: String,
    val options: List<TimezoneOption>,
)

val TIMEZONE_GROUPS = listOf(
    TimezoneGroup(
        "亚洲",
        listOf(
            TimezoneOption("Asia/Shanghai", "中国上海 (UTC+8)"),
            TimezoneOption("Asia/Hong_Kong", "中国香港 (UTC+8)"),
            TimezoneOption("Asia/Taipei", "中国台北 (UTC+8)"),
            TimezoneOption("Asia/Tokyo", "日本东京 (UTC+9)"),
            TimezoneOption("Asia/Seoul", "韩国首尔 (UTC+9)"),
            TimezoneOption("Asia/Singapore", "新加坡 (UTC+8)"),
            TimezoneOption("Asia/Bangkok", "泰国曼谷 (UTC+7)"),
            TimezoneOption("Asia/Kolkata", "印度加尔各答 (UTC+5:30)"),
            TimezoneOption("Asia/Dubai", "阿联酋迪拜 (UTC+4)"),
        ),
    ),
    TimezoneGroup(
        "欧洲",
        listOf(
            TimezoneOption("Europe/London", "英国伦敦 (UTC+0/+1)"),
            TimezoneOption("Europe/Paris", "法国巴黎 (UTC+1/+2)"),
            TimezoneOption("Europe/Berlin", "德国柏林 (UTC+1/+2)"),
            TimezoneOption("Europe/Moscow", "俄罗斯莫斯科 (UTC+3)"),
        ),
    ),
    TimezoneGroup(
        "美洲",
        listOf(
            TimezoneOption("America/New_York", "美国纽约 (UTC-5/-4)"),
            TimezoneOption("America/Chicago", "美国芝加哥 (UTC-6/-5)"),
            TimezoneOption("America/Denver", "美国丹佛 (UTC-7/-6)"),
            TimezoneOption("America/Los_Angeles", "美国洛杉矶 (UTC-8/-7)"),
            TimezoneOption("America/Toronto", "加拿大多伦多 (UTC-5/-4)"),
            TimezoneOption("America/Sao_Paulo", "巴西圣保罗 (UTC-3)"),
        ),
    ),
    TimezoneGroup(
        "大洋洲 / 其他",
        listOf(
            TimezoneOption("Australia/Sydney", "澳大利亚悉尼 (UTC+10/+11)"),
            TimezoneOption("Pacific/Auckland", "新西兰奥克兰 (UTC+12/+13)"),
            TimezoneOption("UTC", "UTC (协调世界时)"),
        ),
    ),
)

fun timezoneLabel(timezone: String): String =
    TIMEZONE_GROUPS.asSequence()
        .flatMap { it.options.asSequence() }
        .firstOrNull { it.value == timezone }
        ?.label ?: timezone
