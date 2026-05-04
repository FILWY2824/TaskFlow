package com.example.taskflow.data.repository

import com.example.taskflow.data.remote.ApiErrorBody
import com.squareup.moshi.JsonAdapter
import com.squareup.moshi.Moshi
import retrofit2.HttpException
import retrofit2.Response
import java.io.IOException

/**
 * 标准化 Repository 层返回:Success(data) / Error(code, message)。
 * Repository 调用 Retrofit 时统一过 [unwrap],把 Response 拆成 Result<T>。
 */
sealed class Result<out T> {
    data class Success<T>(val data: T) : Result<T>()
    data class Error(val code: String, val message: String, val httpStatus: Int = 0) : Result<Nothing>()

    inline fun <R> map(transform: (T) -> R): Result<R> = when (this) {
        is Success -> Success(transform(data))
        is Error -> this
    }

    fun getOrNull(): T? = (this as? Success)?.data

    val isSuccess: Boolean get() = this is Success
    val isError: Boolean get() = this is Error
}

/** 把 Retrofit Response 拆成 Result。null body 返回 Success(Unit) — Retrofit 在 204 上 body 为 null。 */
fun <T> Response<T>.unwrap(moshi: Moshi, errorAdapter: JsonAdapter<ApiErrorBody> = moshi.adapter(ApiErrorBody::class.java)): Result<T> {
    return if (isSuccessful) {
        @Suppress("UNCHECKED_CAST")
        val body = body() ?: Unit as T
        Result.Success(body)
    } else {
        val raw = errorBody()?.string()
        val parsed = raw?.let { runCatching { errorAdapter.fromJson(it) }.getOrNull() }
        if (parsed != null) {
            Result.Error(parsed.error.code, localizedApiMessage(parsed.error.code, parsed.error.message), code())
        } else {
            Result.Error("http_${code()}", "服务端返回错误（HTTP ${code()}），请稍后重试。", code())
        }
    }
}

/** Wraps a network call to convert IOException / HttpException into Result.Error. */
suspend inline fun <T> safeCall(moshi: Moshi, block: () -> Response<T>): Result<T> = try {
    block().unwrap(moshi)
} catch (e: HttpException) {
    Result.Error("http_${e.code()}", "服务端返回错误（HTTP ${e.code()}），请稍后重试。", e.code())
} catch (e: IOException) {
    Result.Error("network", "网络连接失败，请检查网络后重试。")
} catch (e: Exception) {
    Result.Error("unexpected", "发生未知错误：${e.message ?: "请稍后重试"}")
}

fun localizedApiMessage(code: String, message: String): String = when (code) {
    "bad_request" -> "请求内容不正确，请检查输入后重试。"
    "unauthorized" -> "登录状态已失效，请重新登录。"
    "invalid_credentials" -> "邮箱或密码不正确。"
    "invalid_refresh_token" -> "登录状态已过期，请重新登录。"
    "account_disabled" -> "账号已被禁用，请联系管理员。"
    "email_taken" -> "该邮箱已注册。"
    "local_auth_disabled" -> "本地邮箱登录已关闭，请通过认证中心登录。"
    "missing_device_id" -> "登录设备标识缺失，请重新发起登录。"
    "invalid_handoff" -> "登录凭证已失效，请重新登录。"
    "timeout" -> "操作超时，请重试。"
    "poll_failed" -> "登录状态获取失败，请重新尝试。"
    "network" -> "网络连接失败，请检查网络后重试。"
    else -> message.takeIf { it.any { ch -> ch in '\u4e00'..'\u9fff' } }
        ?: "操作失败（$code），请稍后重试。"
}
