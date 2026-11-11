import java.util.Properties

plugins {
    id("com.android.application")
    // The Flutter Gradle Plugin must be applied after the Android and Kotlin Gradle plugins.
    id("dev.flutter.flutter-gradle-plugin")
}

// The upload key that signs release builds (docs/mobile/android-release.md). Read from android/key.properties on a
// developer's machine, else from the environment in CI; never committed. Without it a release build stops: it is
// never signed with the debug key.
val keyProperties = Properties().apply {
    val file = rootProject.file("key.properties")
    if (file.exists()) file.inputStream().use { load(it) }
}

fun signingValue(property: String, env: String): String? =
    keyProperties.getProperty(property)?.takeIf { it.isNotBlank() } ?: System.getenv(env)?.takeIf { it.isNotBlank() }

val uploadKeystore = signingValue("storeFile", "ANDROID_KEYSTORE_PATH")
val uploadKeystorePassword = signingValue("storePassword", "ANDROID_KEYSTORE_PASSWORD")
val uploadKeyAlias = signingValue("keyAlias", "ANDROID_KEY_ALIAS")
val uploadKeyPassword = signingValue("keyPassword", "ANDROID_KEY_PASSWORD")
val uploadKeyConfigured = listOf(uploadKeystore, uploadKeystorePassword, uploadKeyAlias, uploadKeyPassword).all { it != null }

android {
    namespace = "ke.co.fourdraws.app"
    // Pinned (Flutter 3.47.7's defaults) so that a Flutter upgrade cannot change them unnoticed: Google Play requires
    // new apps and updates to target API 36 (recheck at each submission).
    compileSdk = 36
    ndkVersion = flutter.ndkVersion

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    defaultConfig {
        // The store identity of the app: never changes once published.
        applicationId = "ke.co.fourdraws.app"
        minSdk = flutter.minSdkVersion
        targetSdk = 36
        // From --build-number and --build-name (CI passes the run number and the tag), else pubspec.yaml's version.
        versionCode = flutter.versionCode
        versionName = flutter.versionName
    }

    signingConfigs {
        create("release") {
            if (uploadKeyConfigured) {
                storeFile = file(uploadKeystore!!)
                storePassword = uploadKeystorePassword
                keyAlias = uploadKeyAlias
                keyPassword = uploadKeyPassword
            }
        }
    }

    buildTypes {
        release {
            signingConfig = signingConfigs.getByName("release")
        }
    }
}

// A release build without the upload key stops here, before anything is built.
gradle.taskGraph.whenReady {
    val release = allTasks.any { it.project == project && it.name.contains("Release") }
    if (release && !uploadKeyConfigured) {
        throw GradleException(
            "Release builds are signed with the upload key: set android/key.properties (storeFile, storePassword, " +
                "keyAlias, keyPassword) or ANDROID_KEYSTORE_PATH, ANDROID_KEYSTORE_PASSWORD, ANDROID_KEY_ALIAS and " +
                "ANDROID_KEY_PASSWORD. See docs/mobile/android-release.md.",
        )
    }
}

kotlin {
    compilerOptions {
        jvmTarget = org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17
    }
}

flutter {
    source = "../.."
}
