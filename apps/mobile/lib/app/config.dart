import 'package:flutter/foundation.dart';

/// Build-time configuration. Nothing secret lives here: the app holds no credentials, only where the API is.
///
///     flutter run --dart-define=API_BASE_URL=http://localhost:8080
///     flutter build web --dart-define=API_BASE_URL=https://lottery-api.example
class AppConfig {
  const AppConfig({required this.apiBaseUrl});

  /// Reads `API_BASE_URL`. Debug builds default to the local stack; release builds must name it (no default
  /// production URL is compiled in), and plain http is refused outside localhost.
  static AppConfig? fromEnvironment() {
    const raw = String.fromEnvironment('API_BASE_URL');
    final value = raw.isEmpty && kDebugMode ? 'http://localhost:8080' : raw;
    final uri = Uri.tryParse(value);
    if (uri == null || !uri.hasAuthority) return null;
    final local = uri.host == 'localhost' || uri.host == '127.0.0.1' || uri.host == '10.0.2.2';
    if (uri.scheme != 'https' && !(uri.scheme == 'http' && local)) return null;
    return AppConfig(apiBaseUrl: uri);
  }

  final Uri apiBaseUrl;
}
