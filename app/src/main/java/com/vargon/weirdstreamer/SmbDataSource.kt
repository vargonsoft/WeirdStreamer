package com.vargon.weirdstreamer

import android.net.Uri
import androidx.media3.common.C
import androidx.media3.common.util.UnstableApi
import androidx.media3.datasource.BaseDataSource
import androidx.media3.datasource.DataSource
import androidx.media3.datasource.DataSpec
import jcifs.CIFSContext
import jcifs.smb.NtlmPasswordAuthenticator
import jcifs.smb.SmbFile
import jcifs.smb.SmbRandomAccessFile
import java.io.IOException

@UnstableApi
class SmbDataSource(
    private val credentialManager: CredentialManager,
    private val baseContext: CIFSContext
) : BaseDataSource(false) {

    private var raf: SmbRandomAccessFile? = null
    private var uri: Uri? = null
    private var bytesRemaining: Long = 0
    private var opened = false

    override fun open(dataSpec: DataSpec): Long {
        uri = dataSpec.uri
        val path = dataSpec.uri.toString()
        val host = try { path.removePrefix("smb://").substringBefore("/") } catch (e: Exception) { 
            throw IOException("Invalid SMB URI")
        }

        val creds = credentialManager.getCredentials(host)
        val auth = if (creds.first != null && creds.second != null) {
            NtlmPasswordAuthenticator(null, creds.first, creds.second)
        } else {
            NtlmPasswordAuthenticator(null, null, null)
        }

        val context = baseContext.withCredentials(auth)
        val smbFile = SmbFile(path, context)
        
        transferInitializing(dataSpec)
        return try {
            val currentRaf = smbFile.openRandomAccess("r")
            raf = currentRaf
            
            val fileLength = currentRaf.length()
            if (dataSpec.position > fileLength) {
                throw IOException("Position out of range")
            }
            
            currentRaf.seek(dataSpec.position)
            bytesRemaining = if (dataSpec.length != C.LENGTH_UNSET.toLong()) {
                dataSpec.length
            } else {
                fileLength - dataSpec.position
            }
            
            opened = true
            transferStarted(dataSpec)
            bytesRemaining
        } catch (e: IOException) {
            throw e
        }
    }

    override fun read(buffer: ByteArray, offset: Int, length: Int): Int {
        if (length == 0) return 0
        if (bytesRemaining == 0L) return C.RESULT_END_OF_INPUT

        val bytesToRead = if (bytesRemaining == C.LENGTH_UNSET.toLong()) {
            length
        } else {
            Math.min(bytesRemaining, length.toLong()).toInt()
        }

        return try {
            val bytesRead = raf?.read(buffer, offset, bytesToRead) ?: -1
            if (bytesRead == -1) {
                if (bytesRemaining != C.LENGTH_UNSET.toLong() && bytesRemaining != 0L) {
                    throw IOException("End of stream reached before reading all bytes")
                }
                return C.RESULT_END_OF_INPUT
            }
            if (bytesRemaining != C.LENGTH_UNSET.toLong()) {
                bytesRemaining -= bytesRead
            }
            bytesTransferred(bytesRead)
            bytesRead
        } catch (e: IOException) {
            throw e
        }
    }

    override fun getUri(): Uri? = uri

    override fun close() {
        uri = null
        try {
            raf?.close()
        } catch (e: IOException) {
            // Ignore
        } finally {
            raf = null
            if (opened) {
                opened = false
                transferEnded()
            }
        }
    }
}

@UnstableApi
class SmbDataSourceFactory(
    private val credentialManager: CredentialManager,
    private val baseContext: CIFSContext
) : DataSource.Factory {
    override fun createDataSource(): DataSource {
        return SmbDataSource(credentialManager, baseContext)
    }
}
