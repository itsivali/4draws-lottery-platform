import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

/// S7, iOS release readiness (docs/mobile/ios-release.md): what the App Store build is made of, checked on every test
/// run because it cannot be built on Linux. Paths are relative to apps/mobile.
void main() {
  final plist = File('ios/Runner/Info.plist').readAsStringSync();
  final project = File('ios/Runner.xcodeproj/project.pbxproj').readAsStringSync();

  /// The value that follows `<key>name</key>` in Info.plist (a string, true or false).
  String? value(String name) {
    final m = RegExp('<key>$name</key>\\s*<(string|true|false)\\s*/?>([^<]*)').firstMatch(plist);
    if (m == null) return null;
    return m.group(1) == 'string' ? m.group(2) : m.group(1);
  }

  test('the bundle is named Tajirika and declares only exempt encryption', () {
    expect(value('CFBundleName'), 'Tajirika');
    expect(value('CFBundleDisplayName'), 'Tajirika');
    expect(value('ITSAppUsesNonExemptEncryption'), 'false');
  });

  test('iPhone only, in portrait, as the app is designed', () {
    expect(project, isNot(contains('TARGETED_DEVICE_FAMILY = "1,2"')));
    expect(RegExp('TARGETED_DEVICE_FAMILY = 1;').allMatches(project).length, 3, reason: 'debug, release, profile');
    final orientations = RegExp(
      r'<key>UISupportedInterfaceOrientations</key>\s*<array>(.*?)</array>',
      dotAll: true,
    ).firstMatch(plist)!.group(1)!;
    expect(orientations.trim(), '<string>UIInterfaceOrientationPortrait</string>');
    expect(plist, isNot(contains('UISupportedInterfaceOrientations~ipad')));
  });

  test("Google can return to the app through its reversed client id's URL scheme", () {
    expect(plist, contains('<key>CFBundleURLTypes</key>'));
    expect(plist, contains(r'<string>$(GOOGLE_REVERSED_CLIENT_ID)</string>'));
    final base = File('ios/Flutter/App.xcconfig').readAsStringSync();
    expect(base, contains('GOOGLE_REVERSED_CLIENT_ID ='), reason: 'a valid placeholder until the owner sets theirs');
  });

  test("the owner's team and Google client come from an uncommitted Local.xcconfig", () {
    for (final f in ['ios/Flutter/Debug.xcconfig', 'ios/Flutter/Release.xcconfig']) {
      final x = File(f).readAsStringSync();
      expect(x, contains('#include "App.xcconfig"'), reason: f);
      expect(x, contains('#include? "Local.xcconfig"'), reason: f);
    }
    expect(File('ios/Flutter/Local.xcconfig.example').existsSync(), isTrue);
    expect(File('ios/.gitignore').readAsStringSync(), contains('Flutter/Local.xcconfig'));
    expect(project, isNot(contains('DEVELOPMENT_TEAM = ')), reason: 'no team id in the repository');
  });

  test('the privacy manifest exists, is bundled, and declares no tracking', () {
    final manifest = File('ios/Runner/PrivacyInfo.xcprivacy').readAsStringSync();
    expect(manifest, contains('<key>NSPrivacyTracking</key>\n\t<false/>'));
    expect(manifest, contains('NSPrivacyAccessedAPICategoryUserDefaults'));
    expect(manifest, contains('CA92.1'));
    expect(manifest, contains('NSPrivacyCollectedDataTypePhoneNumber'));
    expect(project, contains('PrivacyInfo.xcprivacy in Resources'));
  });
}
