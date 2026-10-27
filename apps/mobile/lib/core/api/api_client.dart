import 'dart:async';
import 'dart:convert';

import 'package:http/http.dart' as http;

import 'api_failure.dart';

/// Supplies access tokens; the session manager implements it.
abstract interface class AccessTokenSource {
  /// A token that is valid now (refreshed first when about to expire), or null when signed out.
  Future<String?> accessToken();

  /// Called after the API refused [rejected]; returns a new token, or null when the session is over.
  Future<String?> refreshAfterRejection(String rejected);
}

class ApiResponse {
  const ApiResponse(this.status, this.json, this.headers);

  final int status;
  final Map<String, Object?> json;
  final Map<String, String> headers;

  /// The API replayed the stored answer to an earlier request with the same idempotency key.
  bool get replayed => headers['idempotent-replay'] == 'true';
}

/// The one way the app talks to the Go API. Writes must name their idempotency key; a 401 is retried once after a
/// refresh with that same key. Nothing here retries anything else: callers that may retry (reads, reconciliation of
/// an uncertain write) decide that themselves.
class ApiClient {
  ApiClient({
    required this.baseUrl,
    required this._http,
    required this._tokens,
    this.timeout = const Duration(seconds: 20),
  });

  final Uri baseUrl;
  final http.Client _http;
  final AccessTokenSource _tokens;
  final Duration timeout;

  Future<ApiResponse> get(String path, {Map<String, String>? query, bool authenticated = true}) =>
      _send('GET', path, query: query, authenticated: authenticated);

  Future<ApiResponse> post(String path, {required String idempotencyKey, Object? body, bool authenticated = true}) =>
      _send('POST', path, body: body, idempotencyKey: idempotencyKey, authenticated: authenticated);

  Future<ApiResponse> put(String path, {required String idempotencyKey, Object? body}) =>
      _send('PUT', path, body: body, idempotencyKey: idempotencyKey, authenticated: true);

  Future<ApiResponse> _send(
    String method,
    String path, {
    Map<String, String>? query,
    Object? body,
    String? idempotencyKey,
    required bool authenticated,
  }) async {
    var token = authenticated ? await _tokens.accessToken() : null;
    if (authenticated && token == null) throw const SessionEndedFailure();
    var res = await _once(method, path, query, body, idempotencyKey, token);
    if (res.statusCode == 401 && token != null) {
      token = await _tokens.refreshAfterRejection(token);
      if (token == null) throw const SessionEndedFailure();
      res = await _once(method, path, query, body, idempotencyKey, token);
      if (res.statusCode == 401) throw const SessionEndedFailure();
    }
    return _decode(res);
  }

  Future<http.Response> _once(
    String method,
    String path,
    Map<String, String>? query,
    Object? body,
    String? idempotencyKey,
    String? token,
  ) async {
    // the base URL may carry a path (a reverse proxy's prefix): the route is appended to it, never swapped for it
    final prefix = baseUrl.path.endsWith('/') ? baseUrl.path.substring(0, baseUrl.path.length - 1) : baseUrl.path;
    final req = http.Request(method, baseUrl.replace(path: '$prefix$path', queryParameters: query));
    req.headers['Accept'] = 'application/json, application/problem+json';
    if (token != null) req.headers['Authorization'] = 'Bearer $token';
    if (idempotencyKey != null) req.headers['Idempotency-Key'] = idempotencyKey;
    if (body != null) {
      req.headers['Content-Type'] = 'application/json';
      req.body = jsonEncode(body);
    }
    try {
      final streamed = await _http.send(req).timeout(timeout);
      return await http.Response.fromStream(streamed).timeout(timeout);
    } on TimeoutException catch (e) {
      throw NetworkFailure(timedOut: true, cause: e);
    } on http.ClientException catch (e) {
      throw NetworkFailure(cause: e);
    }
  }

  ApiResponse _decode(http.Response res) {
    final status = res.statusCode;
    if (status >= 200 && status < 300) {
      if (res.body.isEmpty) return ApiResponse(status, const {}, res.headers);
      return ApiResponse(status, _object(res, status), res.headers);
    }
    final Map<String, Object?> p;
    try {
      p = _object(res, status);
    } on MalformedResponseFailure {
      throw ProblemFailure(status: status, slug: 'http-$status', title: 'Unexpected answer from the server');
    }
    final type = p['type'];
    final retry = int.tryParse(res.headers['retry-after'] ?? '');
    throw ProblemFailure(
      status: status,
      slug: type is String ? Uri.tryParse(type)?.pathSegments.lastOrNull ?? type : 'http-$status',
      title: p['title'] is String ? p['title']! as String : 'Request failed',
      detail: p['detail'] is String ? p['detail']! as String : null,
      retryAfter: retry == null ? null : Duration(seconds: retry),
    );
  }

  Map<String, Object?> _object(http.Response res, int status) {
    try {
      final v = jsonDecode(res.body);
      if (v is Map<String, Object?>) return v;
      throw MalformedResponseFailure(status, 'not a JSON object');
    } on FormatException catch (e) {
      throw MalformedResponseFailure(status, e.message);
    }
  }
}
