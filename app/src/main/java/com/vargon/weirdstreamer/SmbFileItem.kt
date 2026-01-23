package com.vargon.weirdstreamer

data class SmbFileItem(
    val name: String,
    val path: String,
    val isDirectory: Boolean,
    val size: Long = 0
)
