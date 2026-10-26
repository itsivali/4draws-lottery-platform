import 'package:flutter_secure_storage/flutter_secure_storage.dart';

/// Where the refresh token lives between launches. Access tokens are never stored.
abstract interface class TokenStore {
  Future<String?> read();
  Future<void> write(String refreshToken);
  Future<void> clear();
}

/// iOS Keychain / Android Keystore-backed storage, for the phone apps.
class SecureTokenStore implements TokenStore {
  SecureTokenStore([FlutterSecureStorage? storage]) : _storage = storage ?? const FlutterSecureStorage();

  static const _key = 'refresh_token';
  final FlutterSecureStorage _storage;

  @override
  Future<String?> read() => _storage.read(key: _key);

  @override
  Future<void> write(String refreshToken) => _storage.write(key: _key, value: refreshToken);

  @override
  Future<void> clear() => _storage.delete(key: _key);
}

/// Memory only: the web (a browser has no storage a script on the page cannot read, so the web app signs in again
/// after a reload) and tests.
class MemoryTokenStore implements TokenStore {
  String? value;

  @override
  Future<String?> read() async => value;

  @override
  Future<void> write(String refreshToken) async => value = refreshToken;

  @override
  Future<void> clear() async => value = null;
}
