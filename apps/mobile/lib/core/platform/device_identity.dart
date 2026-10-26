import 'package:flutter/foundation.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import '../api/idempotency.dart';
import '../auth/session_manager.dart';

/// Loads (or creates once) this install's random fingerprint. It identifies the install to the API's device limits;
/// it is not derived from any hardware id and means nothing outside this app. The web has none.
Future<DeviceIdentity> loadDeviceIdentity([FlutterSecureStorage storage = const FlutterSecureStorage()]) async {
  if (kIsWeb) return const DeviceIdentity(platform: 'web');
  final platform = defaultTargetPlatform == TargetPlatform.iOS ? 'ios' : 'android';
  const key = 'device_fingerprint';
  var fp = await storage.read(key: key);
  if (fp == null) {
    fp = newIdempotencyKey().replaceFirst('app-', 'dev-');
    await storage.write(key: key, value: fp);
  }
  return DeviceIdentity(platform: platform, fingerprint: fp);
}
