import 'dart:async';
import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;

import '../api/api_client.dart';
import '../api/api_failure.dart';
import '../api/idempotency.dart';
import 'token_store.dart';

enum SessionStatus { restoring, offline, signedIn, signedOut }

/// Why the last session ended: the player signed out, it expired, or the player self-excluded (the server ends every
/// session then, ADR 0030).
enum SessionEndReason { signedOut, expired, selfExcluded }

/// How the API identifies this install: the platform, and on phones a random per-install fingerprint (the web has
/// none; the API limits it by address instead).
class DeviceIdentity {
  const DeviceIdentity({required this.platform, this.fingerprint});

  final String platform;
  final String? fingerprint;

  Map<String, Object?> toJson() => {'platform': platform, 'fingerprint': ?fingerprint};
}

class Tokens {
  const Tokens({required this.access, required this.refresh, required this.expiresAt, this.mustChangePassword = false});

  factory Tokens.fromJson(Map<String, Object?> j) {
    final a = j['access_token'], r = j['refresh_token'], e = j['expires_at'];
    if (a is! String || r is! String || e is! String) throw const FormatException('tokens');
    return Tokens(
      access: a,
      refresh: r,
      expiresAt: DateTime.parse(e),
      mustChangePassword: j['must_change_password'] == true,
    );
  }

  final String access;
  final String refresh;
  final DateTime expiresAt;

  /// The player signed in with a temporary password staff gave them: a new one comes before anything else.
  final bool mustChangePassword;
}

/// Owns the session: keeps the access token in memory and the refresh token in [TokenStore], refreshes shortly before
/// expiry, and makes every refresh single-flight, because presenting a refresh token twice revokes the whole session.
class SessionManager implements AccessTokenSource {
  SessionManager({
    required this.baseUrl,
    required this._http,
    required this._store,
    required this.device,
    DateTime Function()? now,
    this.timeout = const Duration(seconds: 20),
  }) : _now = now ?? DateTime.now;

  static const _refreshMargin = Duration(seconds: 30);

  final Uri baseUrl;
  final DeviceIdentity device;
  final Duration timeout;
  final http.Client _http;
  final TokenStore _store;
  final DateTime Function() _now;

  final status = ValueNotifier(SessionStatus.restoring);
  SessionEndReason? endReason;

  /// True while the player must replace a temporary password (set at sign-in, or when the account says so); the
  /// router keeps them on the password screen until [passwordChanged].
  final mustChangePassword = ValueNotifier(false);

  void passwordChanged() => mustChangePassword.value = false;

  Tokens? _tokens;
  Future<String?>? _refreshing;

  Future<void> restore() async {
    final String? stored;
    try {
      stored = await _store.read();
    } on Object {
      // the secure store could not be read (a platform fault): the app must not hang on "starting"; signing in
      // again writes a fresh token
      status.value = SessionStatus.signedOut;
      return;
    }
    if (stored == null) {
      status.value = SessionStatus.signedOut;
      return;
    }
    _tokens = Tokens(access: '', refresh: stored, expiresAt: DateTime.fromMillisecondsSinceEpoch(0, isUtc: true));
    try {
      await _refresh();
    } on ApiFailure {
      status.value = SessionStatus.offline; // keep the stored token: try again when the network is back
    }
  }

  Future<void> signedIn(Tokens t) async {
    _tokens = t;
    endReason = null;
    mustChangePassword.value = t.mustChangePassword;
    await _store.write(t.refresh);
    status.value = SessionStatus.signedIn;
  }

  @override
  Future<String?> accessToken() async {
    final t = _tokens;
    if (t == null) return null;
    if (t.expiresAt.difference(_now()) > _refreshMargin) return t.access;
    return _refresh();
  }

  @override
  Future<String?> refreshAfterRejection(String rejected) async {
    final t = _tokens;
    if (t == null) return null;
    if (_refreshing != null) return _refreshing;
    if (t.access != rejected) return t.access; // someone already refreshed
    return _refresh();
  }

  Future<String?> _refresh() => _refreshing ??= _doRefresh().whenComplete(() => _refreshing = null);

  Future<String?> _doRefresh() async {
    final current = _tokens;
    if (current == null) return null;
    final key = newIdempotencyKey(); // one key for the retry too, so a lost answer is replayed, not re-run
    http.Response? res;
    for (var attempt = 0; attempt < 2 && res == null; attempt++) {
      try {
        res = await _post('/v1/auth/refresh', key, {
          'refresh_token': current.refresh,
          'device_fingerprint': ?device.fingerprint,
        });
      } on NetworkFailure {
        if (attempt == 1) rethrow;
      }
    }
    if (res!.statusCode == 200) {
      final Tokens next;
      try {
        next = Tokens.fromJson(jsonDecode(res.body) as Map<String, Object?>);
      } on Object {
        throw MalformedResponseFailure(res.statusCode, 'refresh');
      }
      await signedIn(next);
      return next.access;
    }
    if (res.statusCode == 401) {
      await _end(SessionEndReason.expired);
      return null;
    }
    throw ProblemFailure(status: res.statusCode, slug: 'http-${res.statusCode}', title: 'Could not renew the session');
  }

  Future<void> signOut() async {
    final t = _tokens;
    if (t != null && t.access.isNotEmpty) {
      try {
        await _post('/v1/auth/logout', newIdempotencyKey(), null, bearer: t.access);
      } on ApiFailure {
        // the session ends on this device either way; the server's copy expires with the refresh token
      }
    }
    await _end(SessionEndReason.signedOut);
  }

  /// Forgets the session on this device without asking the server, which has ended it already (a self-exclusion).
  Future<void> endedByServer(SessionEndReason reason) => _end(reason);

  Future<void> _end(SessionEndReason reason) async {
    mustChangePassword.value = false;
    _tokens = null;
    endReason = reason;
    await _store.clear();
    status.value = SessionStatus.signedOut;
  }

  Future<http.Response> _post(String path, String key, Map<String, Object?>? body, {String? bearer}) async {
    try {
      return await _http
          .post(
            baseUrl.replace(path: path),
            headers: {
              'Content-Type': 'application/json',
              'Accept': 'application/json',
              'Idempotency-Key': key,
              'Authorization': ?(bearer == null ? null : 'Bearer $bearer'),
            },
            body: body == null ? null : jsonEncode(body),
          )
          .timeout(timeout);
    } on TimeoutException catch (e) {
      throw NetworkFailure(timedOut: true, cause: e);
    } on http.ClientException catch (e) {
      throw NetworkFailure(cause: e);
    }
  }

  void dispose() => status.dispose();
}
