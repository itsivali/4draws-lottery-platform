import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:http/http.dart' as http;

import '../core/api/api_client.dart';
import '../core/auth/session_manager.dart';
import '../core/auth/token_store.dart';
import 'config.dart';

/// Overridden in main() with the build's configuration (and in tests).
final appConfigProvider = Provider<AppConfig>((ref) => throw StateError('AppConfig not provided'));

final httpClientProvider = Provider<http.Client>((ref) {
  final c = http.Client();
  ref.onDispose(c.close);
  return c;
});

final tokenStoreProvider = Provider<TokenStore>((ref) => kIsWeb ? MemoryTokenStore() : SecureTokenStore());

/// Overridden in main() once the install's fingerprint is loaded.
final deviceIdentityProvider = Provider<DeviceIdentity>((ref) => const DeviceIdentity(platform: 'web'));

final sessionManagerProvider = Provider<SessionManager>((ref) {
  final s = SessionManager(
    baseUrl: ref.watch(appConfigProvider).apiBaseUrl,
    http: ref.watch(httpClientProvider),
    store: ref.watch(tokenStoreProvider),
    device: ref.watch(deviceIdentityProvider),
  );
  ref.onDispose(s.dispose);
  return s;
});

final apiClientProvider = Provider<ApiClient>(
  (ref) => ApiClient(
    baseUrl: ref.watch(appConfigProvider).apiBaseUrl,
    http: ref.watch(httpClientProvider),
    tokens: ref.watch(sessionManagerProvider),
  ),
);
