package com.example.taskflow.util

import java.time.Instant
import java.time.LocalDate
import java.time.LocalDateTime
import java.time.ZoneId
import java.time.ZonedDateTime
import java.time.format.DateTimeFormatter
import java.time.format.DateTimeParseException

/**
 * 日期 / 时间格式化辅助。服务端用 RFC3339 UTC,本地展示用用户时区。
 */
object DateTimeFmt {
    private val DATE_FMT: DateTimeFormatter = DateTimeFormatter.ofPattern("yyyy-MM-dd")
    private val DATETIME_FMT: DateTimeFormatter = DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm")
    private val TIME_FMT: DateTimeFormatter = DateTimeFormatter.ofPattern("HH:mm")

    fun localDateTime(iso: String?, tz: String = "UTC"): String {
        if (iso.isNullOrBlank()) return ""
        return try {
            val zoned = ZonedDateTime.ofInstant(Instant.parse(iso), zoneOf(tz))
            DATETIME_FMT.format(zoned)
        } catch (_: DateTimeParseException) {
            iso
        }
    }

    fun localDate(iso: String?, tz: String = "UTC"): String {
        if (iso.isNullOrBlank()) return ""
        return try {
            val zoned = ZonedDateTime.ofInstant(Instant.parse(iso), zoneOf(tz))
            DATE_FMT.format(zoned)
        } catch (_: DateTimeParseException) {
            iso
        }
    }

    fun localTime(iso: String?, tz: String = "UTC"): String {
        if (iso.isNullOrBlank()) return ""
        return try {
            val zoned = ZonedDateTime.ofInstant(Instant.parse(iso), zoneOf(tz))
            TIME_FMT.format(zoned)
        } catch (_: DateTimeParseException) {
            iso
        }
    }

    /** 把"今天 14:30"组合成 RFC3339 UTC */
    fun localToUtcIso(date: LocalDate, hour: Int, minute: Int, tz: String = "UTC"): String {
        val zoned = ZonedDateTime.of(date, java.time.LocalTime.of(hour, minute), zoneOf(tz))
        return zoned.toInstant().toString()
    }

    fun nowLocalDate(tz: String = "UTC"): LocalDate =
        LocalDate.now(zoneOf(tz))

    fun parseLocalDate(s: String): LocalDate = LocalDate.parse(s)

    private fun zoneOf(tz: String): ZoneId =
        try { ZoneId.of(tz) } catch (_: Exception) { ZoneId.systemDefault() }

    fun isOverdue(iso: String?, completedAt: String?): Boolean {
        if (completedAt != null) return false
        if (iso.isNullOrBlank()) return false
        return try {
            Instant.parse(iso).isBefore(Instant.now())
        } catch (_: Exception) { false }
    }

    fun isToday(iso: String?, tz: String = "UTC"): Boolean {
        if (iso.isNullOrBlank()) return false
        return try {
            val today = LocalDate.now(zoneOf(tz))
            val day = ZonedDateTime.ofInstant(Instant.parse(iso), zoneOf(tz)).toLocalDate()
            day == today
        } catch (_: Exception) { false }
    }

    fun relativeFromNow(iso: String?): String {
        if (iso.isNullOrBlank()) return ""
        return try {
            val now = Instant.now()
            val target = Instant.parse(iso)
            val diff = target.epochSecond - now.epochSecond
            val abs = Math.abs(diff)
            val (n, unit) = when {
                abs < 60 -> abs to "秒"
                abs < 3600 -> (abs / 60) to "分钟"
                abs < 86400 -> (abs / 3600) to "小时"
                else -> (abs / 86400) to "天"
            }
            if (diff > 0) "${n}${unit}后" else "${n}${unit}前"
        } catch (_: Exception) { "" }
    }
}
