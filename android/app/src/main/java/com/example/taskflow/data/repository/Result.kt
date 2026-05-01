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
inline fun <T> Response<T>.unwrap(moshi: Moshi, errorAdapter: JsonAdapter<ApiErrorBody> = moshi.adapter(ApiErrorBody::class.java)): Result<T> {
    return if (isSuccessful) {
        @Suppress("UNCHECKED_CAST")
        val body = body() ?: Unit as T
        Result.Success(body)
    } else {
        val raw = errorBody()?.string()
        val parsed = raw?.let { runCatching { errorAdapter.fromJson(it) }.getOrNull() }
        if (parsed != null) {
            Result.Error(parsed.error.code, parsed.error.message, code())
        } else {
            Result.Error("http_${code()}", raw ?: message() ?: "unknown error", code())
        }
    }
}

/** Wraps a network call to convert IOException / HttpException into Result.Error. */
suspend inline fun <T> safeCall(moshi: Moshi, block: () -> Response<T>): Result<T> = try {
    block().unwrap(moshi)
} catch (e: HttpException) {
    Result.Error("http_${e.code()}", e.message ?: "http error", e.code())
} catch (e: IOException) {
    Result.Error("network", e.message ?: "network error")
} catch (e: Exception) {
    Result.Error("unexpected", e.message ?: "unexpected error")
}
