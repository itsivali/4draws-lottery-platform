import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'haptics.dart';

/// Per-device preferences. Nothing here is sensitive, and nothing here changes an outcome.
abstract interface class PreferenceStore {
  Future<bool?> getBool(String key);
  Future<void> setBool(String key, bool value);
}

class SharedPreferenceStore implements PreferenceStore {
  SharedPreferenceStore([SharedPreferencesAsync? prefs]) : _prefs = prefs ?? SharedPreferencesAsync();

  final SharedPreferencesAsync _prefs;

  @override
  Future<bool?> getBool(String key) => _prefs.getBool(key);

  @override
  Future<void> setBool(String key, bool value) => _prefs.setBool(key, value);
}

class MemoryPreferenceStore implements PreferenceStore {
  final values = <String, bool>{};

  @override
  Future<bool?> getBool(String key) async => values[key];

  @override
  Future<void> setBool(String key, bool value) async => values[key] = value;
}

final preferenceStoreProvider = Provider<PreferenceStore>((ref) => SharedPreferenceStore());

/// The *Wheel haptics* setting as stored, loaded before the app starts (so a player who turned haptics off never
/// feels the first spin after a launch). Null when never set.
final storedHapticsPreferenceProvider = Provider<bool?>((ref) => null);

/// Reads the stored setting; main() awaits it and overrides [storedHapticsPreferenceProvider].
Future<bool?> loadStoredHapticsPreference(PreferenceStore store) => store.getBool(HapticsEnabled.key);

/// The *Wheel haptics* setting (on by default where the device supports it).
final hapticsEnabledProvider = NotifierProvider<HapticsEnabled, bool>(HapticsEnabled.new);

class HapticsEnabled extends Notifier<bool> {
  static const key = 'wheel_haptics';

  @override
  bool build() => ref.read(storedHapticsPreferenceProvider) ?? true;

  Future<void> set(bool on) async {
    state = on;
    await ref.read(preferenceStoreProvider).setBool(key, on);
  }
}

/// Whether this platform has device haptics at all (never on the web).
final hapticsSupportedProvider = Provider<bool>((ref) => !kIsWeb);

final hapticsProvider = Provider<Haptics>(
  (ref) =>
      hapticsForPlatform(isWeb: !ref.watch(hapticsSupportedProvider), enabled: () => ref.read(hapticsEnabledProvider)),
);
