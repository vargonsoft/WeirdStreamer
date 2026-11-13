plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
}

val buildNumber: Int = runCatching {
    providers.exec { commandLine("git", "rev-list", "--count", "HEAD") }
        .standardOutput.asText.get().trim().toInt()
}.getOrDefault(1)

android {
    namespace = "com.vargon.weirdstreamer"
    compileSdk = 36

    defaultConfig {
        applicationId = "com.vargon.weirdstreamer"
        minSdk = 29
        targetSdk = 36
        // The build number: the commits in the git history, so each build after a commit is higher
        // (as Android needs for an update to install). The version is set by hand, and the full
        // version name ends with the build number: 0.0.1.254.
        versionCode = buildNumber
        versionName = "0.0.1.$buildNumber"
    }

    signingConfigs {
        // One key for every build, whoever runs it. The default debug key's place depends on the
        // environment ($XDG_CONFIG_HOME/.android when it's set, else ~/.android), and this machine
        // has a different key in each: an update signed with the other key won't install. This
        // is the one the phones, emulators and published builds have.
        getByName("debug") {
            storeFile = file("${System.getProperty("user.home")}/.config/.android/debug.keystore")
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            // Signed with the debug key so release builds can be sideloaded for perf testing.
            signingConfig = signingConfigs.getByName("debug")
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
        }
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_11
        targetCompatibility = JavaVersion.VERSION_11
    }
    buildFeatures {
        compose = true
        buildConfig = true
    }
    // The ASS subtitle renderer (libassrender.so), which native/build-libass.sh builds.
    sourceSets["main"].jniLibs.srcDir(rootProject.file("native/build/jniLibs"))
}

// Builds the ASS renderer for each processor type before the app: libass and its libraries once,
// the renderer again when its source changes.
val buildAssRenderer by tasks.registering(Exec::class) {
    description = "Builds libassrender.so (native/build-libass.sh)"
    inputs.file(file("src/main/cpp/assrender.c"))
    inputs.file(rootProject.file("native/build-libass.sh"))
    outputs.dir(rootProject.file("native/build/jniLibs"))
    commandLine(rootProject.file("native/build-libass.sh").path)
}
tasks.named("preBuild") { dependsOn(buildAssRenderer) }

dependencies {
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.appcompat)
    implementation(platform(libs.androidx.compose.bom))
    implementation(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.ui.graphics)
    implementation(libs.androidx.compose.ui.tooling.preview)
    implementation(libs.androidx.tv.foundation)
    implementation(libs.androidx.tv.material)
    implementation(libs.androidx.lifecycle.runtime.ktx)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(libs.androidx.activity.compose)
    implementation(libs.jcifs.ng)
    implementation(libs.androidx.material3)
    implementation(libs.androidx.material.icons.extended)
    implementation(libs.androidx.security.crypto)
    implementation(libs.coil.compose)
    implementation(libs.coil.video)
    implementation(libs.coil.gif)
    implementation(libs.coil.svg)
    implementation(libs.media3.exoplayer)
    implementation(libs.media3.ui)
    implementation(libs.media3.common)
    implementation(libs.media3.inspector)
    implementation(libs.media3.datasource.okhttp)
    implementation(libs.kotlinx.coroutines.guava)
    implementation(libs.retrofit)
    implementation(libs.retrofit.gson)
    implementation(libs.okhttp)
    implementation(libs.okhttp.logging)
    implementation(libs.okhttp.sse)
    implementation(libs.gson)
    implementation(libs.androidx.work.runtime.ktx)

    androidTestImplementation(platform(libs.androidx.compose.bom))
    androidTestImplementation(libs.androidx.compose.ui.test.junit4)
    debugImplementation(libs.androidx.compose.ui.tooling)
    debugImplementation(libs.androidx.compose.ui.test.manifest)
}
