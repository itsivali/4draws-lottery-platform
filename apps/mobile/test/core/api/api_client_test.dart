import 'dart:async';
import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:lottery_mobile/core/api/api_client.dart';
import 'package:lottery_mobile/core/api/api_failure.dart';
import 'package:lottery_mobile/core/api/idempotency.dart';

class FakeTokens implements AccessTokenSource {
  FakeTokens(this.current, {this.next});

  String? current;
  String? next;
  int refreshes = 0;

  @override
  Future<String?> accessToken() async => current;

  @override
  Future<String?> refreshAfterRejection(String rejected) async {
    refreshes++;
    current = next;
    return next;
  }
}

http.Response json(int status, Object body, {Map<String, String> headers = const {}}) => http.Response(
  jsonEncode(body),
  status,
  headers: {'content-type': status >= 400 ? 'application/problem+json' : 'application/json', ...headers},
);

Map<String, Object?> problem(int status, String slug, String title, [String? detail]) => {
  'type': 'https://lottery.example/problems/$slug',
  'title': title,
  'status': status,
  'detail': ?detail,
};

void main() {
  final base = Uri.parse('https://api.example.test');

  test('sends the bearer token, JSON headers and the idempotency key on writes', () async {
    late http.Request seen;
    final client = ApiClient(
      baseUrl: base,
      http: MockClient((r) async {
        seen = r;
        return json(201, {'ok': true});
      }),
      tokens: FakeTokens('tok-1'),
    );
    final r = await client.post('/v1/plays', idempotencyKey: 'key-1');
    expect(r.status, 201);
    expect(r.json['ok'], true);
    expect(seen.url.toString(), 'https://api.example.test/v1/plays');
    expect(seen.headers['Authorization'], 'Bearer tok-1');
    expect(seen.headers['Idempotency-Key'], 'key-1');
    expect(seen.headers['Accept'], contains('application/json'));
  });

  test('public reads go without a token', () async {
    late http.Request seen;
    final client = ApiClient(
      baseUrl: base,
      http: MockClient((r) async {
        seen = r;
        return json(200, {'draws': <Object>[]});
      }),
      tokens: FakeTokens('tok'),
    );
    await client.get('/v1/draws', query: {'limit': '5'}, authenticated: false);
    expect(seen.headers.containsKey('Authorization'), isFalse);
    expect(seen.url.query, 'limit=5');
  });

  test('turns problem+json into a typed failure with the server title and detail', () async {
    final client = ApiClient(
      baseUrl: base,
      http: MockClient(
        (_) async => json(409, problem(409, 'daily-limit', 'No plays left today', 'plays reset at midnight')),
      ),
      tokens: FakeTokens('t'),
    );
    final f = await client
        .post('/v1/plays', idempotencyKey: 'k')
        .then<ApiFailure?>((_) => null, onError: (Object e) => e as ApiFailure);
    expect(f, isA<ProblemFailure>());
    final p = f! as ProblemFailure;
    expect(p.status, 409);
    expect(p.slug, 'daily-limit');
    expect(p.title, 'No plays left today');
    expect(p.detail, 'plays reset at midnight');
    expect(p.outcomeUncertain, isFalse, reason: 'a definite refusal: nothing was done');
    expect(p.kind, FailureKind.conflict);
  });

  test('classifies failures so screens can explain them', () {
    ProblemFailure p(int s, String slug) => ProblemFailure(status: s, slug: slug, title: 'x');
    expect(p(400, 'invalid-amount').kind, FailureKind.validation);
    expect(p(401, 'invalid-token').kind, FailureKind.authentication);
    expect(p(403, 'kyc-required').kind, FailureKind.eligibility);
    expect(p(404, 'not-found').kind, FailureKind.notFound);
    expect(p(409, 'insufficient-funds').kind, FailureKind.conflict);
    expect(p(429, 'rate-limited').kind, FailureKind.rateLimited);
    expect(p(503, 'game-unavailable').kind, FailureKind.server);
    expect(p(503, 'game-unavailable').outcomeUncertain, isTrue);
    expect(p(409, 'request-in-progress').outcomeUncertain, isTrue, reason: 'the first request is still running');
    expect(p(422, 'idempotency-key-reuse').outcomeUncertain, isFalse);
  });

  test('reads Retry-After from rate limits and unavailability', () async {
    final client = ApiClient(
      baseUrl: base,
      http: MockClient(
        (_) async => json(429, problem(429, 'rate-limited', 'Too many requests'), headers: {'retry-after': '7'}),
      ),
      tokens: FakeTokens('t'),
    );
    final f = await client.get('/v1/plays/today').then<Object?>((_) => null, onError: (Object e) => e);
    expect((f! as ProblemFailure).retryAfter, const Duration(seconds: 7));
  });

  test('a lost answer to a write is uncertain, never a failure', () async {
    final client = ApiClient(
      baseUrl: base,
      http: MockClient((_) async => throw http.ClientException('connection reset')),
      tokens: FakeTokens('t'),
    );
    final f = await client.post('/v1/plays', idempotencyKey: 'k').then<Object?>((_) => null, onError: (Object e) => e);
    expect(f, isA<NetworkFailure>());
    expect((f! as NetworkFailure).outcomeUncertain, isTrue);
  });

  test('a request that takes too long times out as uncertain', () async {
    final never = Completer<http.Response>();
    final client = ApiClient(
      baseUrl: base,
      http: MockClient((_) => never.future),
      tokens: FakeTokens('t'),
      timeout: const Duration(milliseconds: 20),
    );
    final f = await client.post('/v1/plays', idempotencyKey: 'k').then<Object?>((_) => null, onError: (Object e) => e);
    expect(f, isA<NetworkFailure>());
    expect((f! as NetworkFailure).timedOut, isTrue);
  });

  test('a 2xx body that cannot be read is uncertain too (the effect may have happened)', () async {
    final client = ApiClient(
      baseUrl: base,
      http: MockClient((_) async => http.Response('<html>proxy</html>', 201, headers: {'content-type': 'text/html'})),
      tokens: FakeTokens('t'),
    );
    final f = await client.post('/v1/plays', idempotencyKey: 'k').then<Object?>((_) => null, onError: (Object e) => e);
    expect(f, isA<MalformedResponseFailure>());
    expect((f! as ApiFailure).outcomeUncertain, isTrue);
  });

  test('on 401 it refreshes once and retries with the same idempotency key', () async {
    final seen = <http.Request>[];
    final tokens = FakeTokens('old', next: 'new');
    final client = ApiClient(
      baseUrl: base,
      http: MockClient((r) async {
        seen.add(r);
        if (r.headers['Authorization'] == 'Bearer old') {
          return json(401, problem(401, 'invalid-token', 'Invalid token'));
        }
        return json(201, {'ok': true});
      }),
      tokens: tokens,
    );
    final r = await client.post('/v1/plays', idempotencyKey: 'same-key');
    expect(r.status, 201);
    expect(tokens.refreshes, 1);
    expect(seen.map((r) => r.headers['Idempotency-Key']), ['same-key', 'same-key']);
  });

  test('when the session cannot be refreshed the failure says so (and is not retried again)', () async {
    var calls = 0;
    final client = ApiClient(
      baseUrl: base,
      http: MockClient((_) async {
        calls++;
        return json(401, problem(401, 'invalid-token', 'Invalid token'));
      }),
      tokens: FakeTokens('old'),
    );
    final f = await client.get('/v1/plays/today').then<Object?>((_) => null, onError: (Object e) => e);
    expect(f, isA<SessionEndedFailure>());
    expect(calls, 1);
  });

  test('a 204 has an empty body', () async {
    final client = ApiClient(
      baseUrl: base,
      http: MockClient((_) async => http.Response('', 204)),
      tokens: FakeTokens('t'),
    );
    final r = await client.post('/v1/auth/logout', idempotencyKey: 'k');
    expect(r.status, 204);
    expect(r.json, isEmpty);
  });

  test('idempotency keys are unique and match the API pattern', () {
    final keys = {for (var i = 0; i < 1000; i++) newIdempotencyKey()};
    expect(keys.length, 1000);
    final pattern = RegExp(r'^[A-Za-z0-9._:~=+/-]{1,255}$');
    expect(keys.every(pattern.hasMatch), isTrue);
  });
}
