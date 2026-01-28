package com.vargon.weirdstreamer

import coil.annotation.ExperimentalCoilApi
import coil.decode.ImageSource
import jcifs.smb.SmbFile

/**
 * Metadata to pass the SmbFile to the decoder for random access.
 */
@OptIn(ExperimentalCoilApi::class)
class SmbMetadata(val smbFile: SmbFile) : ImageSource.Metadata()
