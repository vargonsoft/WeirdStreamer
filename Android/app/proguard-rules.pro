# Add project specific ProGuard rules here.
# You can control the set of applied configuration files using the
# proguardFiles setting in build.gradle.
#
# For more details, see
#   http://developer.android.com/guide/developing/tools/proguard.html

# If your project uses WebView with JS, uncomment the following
# and specify the fully qualified class name to the JavaScript interface
# class:
#-keepclassmembers class fqcn.of.javascript.interface.for.webview {
#   public *;
#}

# Uncomment this to preserve the line number information for
# debugging stack traces.
#-keepattributes SourceFile,LineNumberTable

# If you keep the line number information, uncomment this to
# hide the original source file name.
#-renamesourcefileattribute SourceFile
-keepattributes SourceFile,LineNumberTable

# Gson-deserialized models (reflection on field names / no-arg construction)
-keep class com.vargon.weirdstreamer.api.models.** { *; }
-keep class com.vargon.weirdstreamer.api.ThumbnailUpdate { *; }

# Retrofit suspend functions need generic signatures
-keepattributes Signature, InnerClasses, EnclosingMethod, *Annotation*
-keep,allowobfuscation,allowshrinking interface retrofit2.Call
-keep,allowobfuscation,allowshrinking class retrofit2.Response
-keep,allowobfuscation,allowshrinking class kotlin.coroutines.Continuation

# jcifs-ng loads protocol/crypto classes reflectively
-keep class jcifs.** { *; }
-keep class org.bouncycastle.** { *; }
-dontwarn jcifs.**
-dontwarn org.bouncycastle.**
-dontwarn org.slf4j.**
-dontwarn javax.servlet.**

# Downloads and offline progress are stored as JSON through Gson, by field name: keep the names
# (and the State enum) so release builds read what earlier builds wrote.
-keep class com.vargon.weirdstreamer.downloads.DownloadItem { *; }
-keep class com.vargon.weirdstreamer.downloads.DownloadItem$State { *; }
-keep class com.vargon.weirdstreamer.downloads.LocalProgress$Entry { *; }

# WorkManager (downloads) creates its Room database reflectively at startup; R8 removed the
# generated database's constructor, crashing every launch.
-keep class * extends androidx.room.RoomDatabase { <init>(); }
-keep class * extends androidx.work.ListenableWorker { <init>(android.content.Context, androidx.work.WorkerParameters); }
