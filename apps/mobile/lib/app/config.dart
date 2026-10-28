import 'package:flutter/foundation.dart';

/// Build-time configuration. Nothing secret lives here: the app holds no credentials, only where the API is.
///
///     flutter run --dart-define=API_BASE_URL=http://localhost:8080
///     flutter build web --dart-define=API_BASE_URL=https://lottery-api.example
class AppConfig {
  const AppConfig({required this.apiBaseUrl, this.backOfficeUrl});

  /// Reads `API_BASE_URL` and, for the web app's link to the staff back office, `BACKOFFICE_URL`. Debug builds default
  /// to the local stack; release builds must name the server (no default production URL is compiled in), and plain
  /// http is refused outside localhost. A bad back-office address is dropped, never guessed.
  static AppConfig? fromEnvironment() {
    const raw = String.fromEnvironment('API_BASE_URL');
    final api = secureUrl(raw.isEmpty && kDebugMode ? 'http://localhost:8080' : raw);
    if (api == null) return null;
    const office = String.fromEnvironment('BACKOFFICE_URL');
    return AppConfig(
      apiBaseUrl: api,
      backOfficeUrl: secureUrl(office.isEmpty && kDebugMode ? 'http://localhost:5173' : office),
    );
  }

  /// A web address the app may point at: https, or http on a local host; null otherwise.
  static Uri? secureUrl(String value) {
    final uri = Uri.tryParse(value);
    if (uri == null || !uri.hasAuthority) return null;
    final local = uri.host == 'localhost' || uri.host == '127.0.0.1' || uri.host == '10.0.2.2';
    if (uri.scheme != 'https' && !(uri.scheme == 'http' && local)) return null;
    return uri;
  }

  final Uri apiBaseUrl;

  /// Where the staff back office lives, for the link the web app shows; null when the build names none.
  final Uri? backOfficeUrl;
}
