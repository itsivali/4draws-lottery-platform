import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

/// S6, Android release readiness (docs/mobile/android-release.md): what the store build is made of, checked on every
/// test run because it cannot be seen in the app. Paths are relative to apps/mobile (where `flutter test` runs).
void main() {
  final gradle = File('android/app/build.gradle.kts').readAsStringSync();
  final manifest = File('android/app/src/main/AndroidManifest.xml').readAsStringSync();

  test('a release build is never signed with the debug key', () {
    expect(gradle, isNot(contains('signingConfigs.getByName("debug")')));
    expect(gradle, contains('create("release")'));
    expect(gradle, contains('signingConfig = signingConfigs.getByName("release")'));
    // without the upload key a release build stops, rather than producing an unsigned or debug-signed bundle
    expect(gradle, contains('throw GradleException'));
    for (final name in [
      'ANDROID_KEYSTORE_PATH',
      'ANDROID_KEYSTORE_PASSWORD',
      'ANDROID_KEY_ALIAS',
      'ANDROID_KEY_PASSWORD',
    ]) {
      expect(gradle, contains(name), reason: name);
    }
  });

  test('the SDK levels are pinned to what Play requires', () {
    expect(gradle, contains('targetSdk = 36'));
    expect(gradle, contains('compileSdk = 36'));
    expect(gradle, isNot(contains('TODO')), reason: "the template's TODOs are resolved");
  });

  test('nothing of the app is backed up or carried to another device', () {
    expect(manifest, contains('android:allowBackup="false"'));
    expect(manifest, contains('android:fullBackupContent="@xml/backup_rules"'));
    expect(manifest, contains('android:dataExtractionRules="@xml/data_extraction_rules"'));
    final extraction = File('android/app/src/main/res/xml/data_extraction_rules.xml').readAsStringSync();
    for (final section in ['<cloud-backup', '<device-transfer']) {
      expect(extraction, contains(section));
    }
    for (final domain in ['root', 'file', 'database', 'sharedpref', 'external']) {
      expect(RegExp('<exclude domain="$domain" path="\\."').allMatches(extraction).length, 2, reason: domain);
    }
    final rules = File('android/app/src/main/res/xml/backup_rules.xml').readAsStringSync();
    for (final domain in ['root', 'file', 'database', 'sharedpref', 'external']) {
      expect(rules, contains('<exclude domain="$domain" path="."/>'), reason: domain);
    }
  });

  test('the version code comes from the build, not the pubspec', () {
    final workflow = File('../../.github/workflows/mobile.yml').readAsStringSync();
    expect(workflow, contains('--build-number='));
  });
}
