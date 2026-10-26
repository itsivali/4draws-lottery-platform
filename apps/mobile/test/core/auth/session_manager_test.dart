import 'dart:async';
import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:lottery_mobile/core/api/api_failure.dart';
import 'package:lottery_mobile/core/auth/session_manager.dart';
import 'package:lottery_mobile/core/auth/token_store.dart';

final base = Uri.parse('https://api.example.test');
final t0 = DateTime.utc(2026, 10, 26, 9);

http.Response tokens(String access, String refresh, DateTime expires) => http.Response(
  jsonEncode({'access_token': access, 'refresh_token': refresh, 'expires_at': expires.toIso8601String()}),
  200,
  headers: {'content-type': 'application/json'},
);

void main() {
  late DateTime now;
  setUp(() => now = t0);

  SessionManager manager(http.Client client, TokenStore store) => SessionManager(
    baseUrl: base,
    http: client,
    store: store,
    device: const DeviceIdentity(platform: 'android', fingerprint: 'fp-1'),
    now: () => now,
  );

  test('signing in keeps the refresh token in the store and the access token in memory', () async {
    final store = MemoryTokenStore();
    final s = manager(MockClient((_) async => throw StateError('no calls')), store);
    await s.signedIn(Tokens(access: 'a1', refresh: 'r1', expiresAt: t0.add(const Duration(minutes: 10))));
    expect(s.status.value, SessionStatus.signedIn);
    expect(await s.accessToken(), 'a1');
    expect(await store.read(), 'r1');
  });

  test('refreshes before the access token expires, sending the device fingerprint', () async {
    final store = MemoryTokenStore();
    late http.Request seen;
    final s = manager(
      MockClient((r) async {
        seen = r;
        return tokens('a2', 'r2', t0.add(const Duration(minutes: 20)));
      }),
      store,
    );
    await s.signedIn(Tokens(access: 'a1', refresh: 'r1', expiresAt: t0.add(const Duration(minutes: 10))));
    now = t0.add(const Duration(minutes: 9, seconds: 45));
    expect(await s.accessToken(), 'a2');
    expect(seen.url.path, '/v1/auth/refresh');
    expect(jsonDecode(seen.body), {'refresh_token': 'r1', 'device_fingerprint': 'fp-1'});
    expect(seen.headers['Idempotency-Key'], isNotEmpty);
    expect(await store.read(), 'r2');
  });

  test('concurrent callers share one refresh (a second use of a refresh token would revoke the session)', () async {
    var calls = 0;
    final gate = Completer<void>();
    final s = manager(
      MockClient((_) async {
        calls++;
        await gate.future;
        return tokens('a2', 'r2', t0.add(const Duration(minutes: 20)));
      }),
      MemoryTokenStore(),
    );
    await s.signedIn(Tokens(access: 'a1', refresh: 'r1', expiresAt: t0));
    final all = Future.wait([s.accessToken(), s.accessToken(), s.refreshAfterRejection('a1')]);
    gate.complete();
    expect(await all, ['a2', 'a2', 'a2']);
    expect(calls, 1);
  });

  test('a rejection of an old token after a refresh does not refresh again', () async {
    var calls = 0;
    final s = manager(
      MockClient((_) async {
        calls++;
        return tokens('a2', 'r2', t0.add(const Duration(minutes: 20)));
      }),
      MemoryTokenStore(),
    );
    await s.signedIn(Tokens(access: 'a1', refresh: 'r1', expiresAt: t0));
    expect(await s.refreshAfterRejection('a1'), 'a2');
    expect(await s.refreshAfterRejection('a1'), 'a2');
    expect(calls, 1);
  });

  test('a refused refresh ends the session and clears the store', () async {
    final store = MemoryTokenStore();
    final s = manager(
      MockClient(
        (_) async => http.Response(
          jsonEncode({
            'type': 'https://lottery.example/problems/invalid-token',
            'title': 'Invalid token',
            'status': 401,
          }),
          401,
        ),
      ),
      store,
    );
    await s.signedIn(Tokens(access: 'a1', refresh: 'r1', expiresAt: t0));
    expect(await s.accessToken(), isNull);
    expect(s.status.value, SessionStatus.signedOut);
    expect(s.endReason, SessionEndReason.expired);
    expect(await store.read(), isNull);
  });

  test('a lost refresh answer is retried once with the same key, then reported without signing out', () async {
    final keys = <String?>[];
    final s = manager(
      MockClient((r) async {
        keys.add(r.headers['Idempotency-Key']);
        throw http.ClientException('offline');
      }),
      MemoryTokenStore(),
    );
    await s.signedIn(Tokens(access: 'a1', refresh: 'r1', expiresAt: t0));
    await expectLater(s.accessToken(), throwsA(isA<NetworkFailure>()));
    expect(keys, hasLength(2));
    expect(keys.toSet(), hasLength(1));
    expect(s.status.value, SessionStatus.signedIn, reason: 'being offline is not being signed out');
  });

  test('restores a stored session at start-up, or reports signed out', () async {
    final store = MemoryTokenStore()..value = 'r1';
    final s = manager(MockClient((_) async => tokens('a2', 'r2', t0.add(const Duration(minutes: 10)))), store);
    expect(s.status.value, SessionStatus.restoring);
    await s.restore();
    expect(s.status.value, SessionStatus.signedIn);
    expect(await s.accessToken(), 'a2');

    final empty = manager(MockClient((_) async => throw StateError('no call')), MemoryTokenStore());
    await empty.restore();
    expect(empty.status.value, SessionStatus.signedOut);
    expect(empty.endReason, isNull, reason: 'never signed in is not an expiry');
  });

  test('restoring while offline keeps the stored token for later', () async {
    final store = MemoryTokenStore()..value = 'r1';
    final s = manager(MockClient((_) async => throw http.ClientException('offline')), store);
    await s.restore();
    expect(s.status.value, SessionStatus.offline);
    expect(await store.read(), 'r1');
  });

  test('signing out tells the server, best effort, and forgets the tokens', () async {
    final store = MemoryTokenStore();
    final paths = <String>[];
    final s = manager(
      MockClient((r) async {
        paths.add(r.url.path);
        return http.Response('', 204);
      }),
      store,
    );
    await s.signedIn(Tokens(access: 'a1', refresh: 'r1', expiresAt: t0.add(const Duration(minutes: 10))));
    await s.signOut();
    expect(paths, ['/v1/auth/logout']);
    expect(await store.read(), isNull);
    expect(s.status.value, SessionStatus.signedOut);
    expect(s.endReason, SessionEndReason.signedOut);
    expect(await s.accessToken(), isNull);
  });
}
