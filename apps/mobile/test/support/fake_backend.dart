import 'dart:convert';

import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'fixtures.dart';

/// An in-memory stand-in for the Go API (api/openapi.yaml), for widget and integration tests. It keeps the same
/// rules that matter to the app: the server picks the slot, plays are idempotent per key, a top-up stays pending until
/// "Daraja" confirms it, refusals come back as problem+json. Nothing like this ships in the app.
class FakeBackend {
  FakeBackend({this.kyc = 'verified', this.phoneVerified = true, this.balance = 1000});

  String kyc;
  bool phoneVerified;
  int balance;
  int debt = 0;

  /// The smallest top-up the "server" accepts (TOP_UP_MIN_KES; KES 1 while testing with real money).
  int topUpMin = 10;
  Map<String, Object?>? restriction;
  String? closureRequestedAt;
  int plays = 0;
  bool lucky = false;
  final tickets = <Map<String, Object?>>[];
  final txs = <Map<String, Object?>>[];

  /// The slots the "server" spins next, in order (then 6 forever).
  final spins = <int>[];

  /// The next play commits but its answer is lost (the client sees a dropped connection).
  bool loseNextPlayAnswer = false;

  /// Every request fails as if offline.
  bool offline = false;

  /// How many status checks a top-up stays pending for before it is credited (null: stays pending).
  int? topUpPendingPolls = 2;
  final topUps = <String, Map<String, Object?>>{};
  final _topUpPolls = <String, int>{};
  final payouts = <String, Map<String, Object?>>{};
  final draws = <Map<String, Object?>>[
    drawJson(id: 'draw-daily', status: 'open'),
    drawJson(
      id: 'draw-done',
      type: 'weekly',
      status: 'approved',
      results: [
        {'rank': 1, 'amount_kes': 5000, 'entry_no': 17},
      ],
    ),
  ];

  final requests = <http.Request>[];
  final _idempotent = <String, http.Response>{};
  int _ids = 0;
  String access = 'access-1';
  bool signedIn = false;

  /// Refresh tokens are refused (the session was revoked).
  bool refreshFails = false;

  late final http.Client client = MockClient(_handle);

  static const _json = {'content-type': 'application/json'};

  http.Response _ok(Object body, [int status = 200]) => http.Response(jsonEncode(body), status, headers: _json);
  http.Response _problem(int status, String slug, String title, [String? detail]) => http.Response(
    jsonEncode(problemJson(status, slug, title, detail)),
    status,
    headers: {'content-type': 'application/problem+json'},
  );

  Future<http.Response> _handle(http.Request r) async {
    requests.add(r);
    if (offline) throw http.ClientException('offline');
    final path = r.url.path;
    final method = r.method;
    final key = r.headers['Idempotency-Key'];
    if (method != 'GET' && (key == null || key.isEmpty)) {
      return _problem(400, 'idempotency-key-required', 'Idempotency-Key required');
    }
    final public =
        path == '/v1/auth/login' ||
        path == '/v1/auth/signup' ||
        path == '/v1/auth/refresh' ||
        (method == 'GET' && path.startsWith('/v1/draws') && !path.endsWith('/mine'));
    if (!public && r.headers['Authorization'] != 'Bearer $access') {
      return _problem(401, 'unauthenticated', 'Authentication required');
    }
    if (key != null && _idempotent.containsKey('$path|$key')) {
      final stored = _idempotent['$path|$key']!;
      return http.Response(
        stored.body,
        stored.statusCode == 201 ? 200 : stored.statusCode,
        headers: {...stored.headers, 'idempotent-replay': 'true'},
      );
    }
    final res = _route(method, path, r);
    if (key != null && res.statusCode < 500) _idempotent['$path|$key'] = res;
    if (path == '/v1/plays' && loseNextPlayAnswer) {
      loseNextPlayAnswer = false;
      throw http.ClientException('connection reset after commit');
    }
    return res;
  }

  Map<String, Object?> get _body => const {};

  http.Response _route(String method, String path, http.Request r) {
    final body = r.body.isEmpty ? _body : jsonDecode(r.body) as Map<String, Object?>;
    final tokens = {
      'access_token': access,
      'refresh_token': 'refresh-1',
      'expires_at': DateTime.now().toUtc().add(const Duration(hours: 1)).toIso8601String(),
    };
    switch ((method, path)) {
      case ('POST', '/v1/auth/login'):
        if (body['password'] != 'correct horse battery') {
          return _problem(401, 'invalid-credentials', 'Invalid phone number or password');
        }
        signedIn = true;
        return _ok(tokens);
      case ('POST', '/v1/auth/signup'):
        signedIn = true;
        return _ok(tokens, 201);
      case ('POST', '/v1/auth/refresh'):
        if (refreshFails) return _problem(401, 'invalid-token', 'Invalid token');
        return _ok(tokens);
      case ('POST', '/v1/auth/logout'):
        signedIn = false;
        return http.Response('', 204);
      case ('GET', '/v1/account'):
        return _ok(
          accountJson(
            kyc: kyc,
            phoneVerified: phoneVerified,
            restriction: restriction,
            closureRequestedAt: closureRequestedAt,
          ),
        );
      case ('POST', '/v1/account/closure'):
        closureRequestedAt ??= '2026-10-26T12:00:00+03:00';
        return _ok({'closure_requested_at': closureRequestedAt}, 202);
      case ('GET', '/v1/kyc'):
        return _ok({'status': kyc});
      case ('POST', '/v1/kyc'):
        kyc = 'manual_review';
        return _ok({'status': kyc}, 202);
      case ('GET', '/v1/plays/today'):
        return _ok(_today());
      case ('POST', '/v1/plays'):
        return _play();
      case ('GET', '/v1/wallet'):
        return _ok({
          'balance_kes': balance,
          'debt_kes': debt,
          'transactions': txs.reversed.toList(),
          'next_before': null,
          'top_up': {'min_kes': topUpMin, 'max_kes': 250000},
        });
      case ('GET', '/v1/payments'):
        return _ok({'payments': topUps.values.toList().reversed.toList()});
      case ('POST', '/v1/payments/topups'):
        if (kyc != 'verified') return _problem(403, 'kyc-required', 'Identity verification required');
        final id = 'pay-${++_ids}';
        final amount = body['amount_kes']! as int;
        if (amount < topUpMin) return _problem(400, 'invalid-amount', 'Invalid amount');
        topUps[id] = topUpJson(id: id, status: 'pending', amount: amount);
        _topUpPolls[id] = 0;
        return _ok(topUps[id]!, 202);
      case ('GET', '/v1/payouts'):
        return _ok({'payouts': payouts.values.toList().reversed.toList()});
      case ('POST', '/v1/payouts/withdrawals'):
        final amount = body['amount_kes']! as int;
        if (amount > balance) return _problem(409, 'insufficient-funds', 'Your balance is below the amount');
        balance -= amount;
        final id = 'po-${++_ids}';
        payouts[id] = payoutJson(id: id, amount: amount);
        return _ok(payouts[id]!, 201);
      case ('GET', '/v1/draws'):
        return _ok({'draws': draws});
      case ('GET', '/v1/rg/limits'):
        return _ok({
          'limits': {
            'daily': {'current': null, 'pending': null},
            'weekly': {'current': null, 'pending': null},
            'monthly': {'current': null, 'pending': null},
          },
        });
      case ('POST', '/v1/rg/timeout'):
        restriction = {'kind': 'timeout', 'until': '2026-10-27T12:00:00+03:00'};
        return http.Response('', 204);
      case ('POST', '/v1/rg/self-exclusion'):
        restriction = {'kind': 'self_exclusion', 'until': null};
        return http.Response('', 204);
      case ('GET', '/v1/fairness'):
        return _ok({
          'active': {
            'id': 'chain-1',
            'commitment': 'c' * 64,
            'client_seed': 'player-seed',
            'next_nonce': plays,
            'revealed_seed': null,
            'created_at': '2026-10-26T08:00:00Z',
            'revealed_at': null,
          },
          'rotated': <Object>[],
        });
    }
    if (method == 'GET' && path.startsWith('/v1/payments/')) {
      final id = path.split('/').last;
      final t = topUps[id];
      if (t == null) return _problem(404, 'not-found', 'Not found');
      final polls = _topUpPolls[id] = _topUpPolls[id]! + 1;
      if (topUpPendingPolls != null && polls >= topUpPendingPolls! && t['status'] == 'pending') {
        final amount = t['amount_requested_kes']! as int;
        topUps[id] = topUpJson(id: id, status: 'credited', amount: amount, confirmed: amount);
        balance += amount; // the operator pays the deposit excise
        phoneVerified = true;
      }
      return _ok(topUps[id]!);
    }
    if (method == 'GET' && path.startsWith('/v1/payouts/')) {
      final p = payouts[path.split('/').last];
      return p == null ? _problem(404, 'not-found', 'No such payout') : _ok(p);
    }
    if (method == 'GET' && path.startsWith('/v1/draws/')) {
      final parts = path.split('/');
      if (parts.length == 5 && parts[4] == 'mine') {
        return _ok({'draw_id': parts[3], 'user_id': 'user-1', 'entries': <Object>[], 'prizes': <Object>[]});
      }
      final d = draws.where((d) => d['id'] == parts[3]).firstOrNull;
      return d == null ? _problem(404, 'not-found', 'No such draw') : _ok(d);
    }
    return _problem(404, 'not-found', 'Not found');
  }

  Map<String, Object?> _today() => {
    'play_date': '2026-10-26',
    'plays': plays,
    'plays_left': 4 - plays,
    'lucky': lucky,
    'chances': _chances(),
    'price_kes': 100,
    'tickets': tickets,
  };

  int _chances() => (const [0, 1, 2, 3, 5][plays]) * (lucky ? 2 : 1);

  http.Response _play() {
    if (restriction != null) return _problem(403, 'restricted', 'Your account is restricted');
    if (kyc != 'verified') return _problem(403, 'kyc-required', 'Identity verification required');
    if (!phoneVerified) {
      return _problem(
        403,
        'phone-not-verified',
        'Phone number not verified yet',
        'make your first top-up to verify it',
      );
    }
    if (plays >= 4) {
      return _problem(
        409,
        'daily-limit',
        'No plays left today',
        'you can play 4 times a day; plays reset at midnight Nairobi time',
      );
    }
    if (balance < 100) {
      return _problem(409, 'insufficient-funds', 'Your balance is below the price of a play', 'top up first');
    }
    plays++;
    balance -= 100;
    final slot = spins.isEmpty ? 6 : spins.removeAt(0);
    if (slot == 0) lucky = true;
    final play = playJson(ticket: 'ticket-$plays', seq: plays, slot: slot, lucky: slot == 0);
    tickets.add(play);
    txs.add({
      'id': 'tx-${++_ids}',
      'kind': 'stake',
      'amount_kes': -100,
      'reference_type': 'ticket',
      'reference_id': 'ticket-$plays',
      'created_at': '2026-10-26T10:00:00+03:00',
    });
    return _ok({'play': play, 'today': _today(), 'balance_kes': balance}, 201);
  }

  int countPlayRequests() => requests.where((r) => r.method == 'POST' && r.url.path == '/v1/plays').length;
}
