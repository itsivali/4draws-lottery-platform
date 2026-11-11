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

  /// The least seconds between two prompts (TOP_UP_COOLDOWN; 0: none), counted from the last top-up started.
  int topUpCooldownS = 0;
  DateTime? _lastTopUp;

  int get _topUpWaitS {
    final last = _lastTopUp;
    if (topUpCooldownS == 0 || last == null) return 0;
    final left = last.add(Duration(seconds: topUpCooldownS)).difference(DateTime.now()).inMilliseconds;
    return left <= 0 ? 0 : (left / 1000).ceil();
  }

  /// The withdrawal range, or null while the "server" has no M-Pesa B2C (withdrawals unavailable).
  Map<String, Object?>? withdrawBounds = {'min_kes': 50, 'max_kes': 150000};
  Map<String, Object?>? restriction;
  String? closureRequestedAt;
  int plays = 0;

  /// The player's own draw periods (GET /v1/draws/periods); empty before the first play.
  List<Map<String, Object?>> periods = [];
  bool lucky = false;
  final tickets = <Map<String, Object?>>[];
  final txs = <Map<String, Object?>>[];

  /// The slots the "server" spins next, in order (then 6 forever).
  final spins = <int>[];

  /// The next play commits but its answer is lost (the client sees a dropped connection).
  /// What the account gave at sign-up (owner, 2026-10-30), as GET /v1/kyc reports it.
  Map<String, Object?>? declared = {'first_name': 'Jane', 'second_name': 'Wanjiru', 'national_id': '12345678'};

  /// Where the declared identity stands, as GET /v1/account reports it: none, declared, submitted or verified.
  String identityStatus = 'declared';

  /// A problem slug POST /v1/account/identity answers with 409 (`identity-declared`, `kyc-state`), or null to accept.
  String? declareRefusal;

  /// The devices signed in (GET /v1/account/sessions), newest first, this one marked.
  final sessions = <Map<String, Object?>>[
    {
      'id': 'sess-2',
      'platform': 'web',
      'device_verdict': 'none',
      'created_at': '2026-10-24T20:00:00+03:00',
      'last_used_at': '2026-10-25T20:30:00+03:00',
      'current': false,
    },
    {
      'id': 'sess-1',
      'platform': 'android',
      'device_verdict': 'passed',
      'created_at': '2026-10-20T09:00:00+03:00',
      'last_used_at': '2026-10-26T08:00:00+03:00',
      'current': true,
    },
  ];

  /// The Google or Apple identities linked to the account (GET /v1/account/sign-ins).
  final linkedSignIns = <Map<String, Object?>>[];

  static bool _identityMissing(Map<String, Object?> body) =>
      [body['first_name'], body['second_name'], body['national_id']].any((v) => v is! String || v.trim().isEmpty);

  bool loseNextPlayAnswer = false;

  /// Every request fails as if offline.
  bool offline = false;

  /// How many status checks a top-up stays pending for before it is credited (null: stays pending).
  int? topUpPendingPolls = 2;

  /// How a top-up ends after [topUpPendingPolls]: `credited` (default) or `unconfirmed` (M-Pesa never confirmed it).
  String topUpOutcome = 'credited';

  /// What M-Pesa says about a receipt the player gives for an uncredited top-up, on the next read: `confirmed`
  /// (credited), `unverified` (left for the team) or `checking` (no answer yet).
  String claimOutcome = 'confirmed';
  final topUps = <String, Map<String, Object?>>{};
  final _topUpPolls = <String, int>{};
  final _claimReads = <String, int>{};

  /// M-Pesa answers about a claimed receipt on the second read after the claim (the first still shows it sent).
  void _settleClaim(String id, {bool count = true}) {
    final t = topUps[id];
    final claim = t?['receipt_claim'] as Map<String, Object?>?;
    if (t == null || claim == null || claim['status'] != 'claimed') return;
    final reads = _claimReads[id] = (_claimReads[id] ?? 0) + (count ? 1 : 0);
    if (reads < 2) return;
    final amount = t['amount_requested_kes']! as int;
    switch (claimOutcome) {
      case 'confirmed':
        topUps[id] = topUpJson(
          id: id,
          status: 'credited',
          amount: amount,
          confirmed: amount,
          claim: {'receipt': claim['receipt'], 'status': 'confirmed'},
        );
        balance += amount;
        phoneVerified = true;
      case 'unverified':
        topUps[id] = {
          ...t,
          'receipt_claim': {
            'receipt': claim['receipt'],
            'status': 'unverified',
            'note': 'M-Pesa did not confirm the receipt: The transaction receipt number does not exist.',
          },
        };
      default:
        topUps[id] = {
          ...t,
          'receipt_claim': {'receipt': claim['receipt'], 'status': 'checking'},
        };
    }
  }

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

  /// What GET /v1/auth/providers answers: Google and Apple on offer, or {} for neither (ADR 0020).
  Map<String, Object?> providers = {
    'google': {'client_id': 'web-client.apps.googleusercontent.com'},
    'apple': {'client_id': 'ke.tajirika.web'},
  };

  /// The Google or Apple ID tokens that already sign in to an account; others get 409 sso-not-linked.
  final ssoAccounts = <String>{};

  /// The account's email address (null for one from before addresses were asked for) and whether it has a password.
  String? email = 'jane@example.com';

  /// The player's names, from the identity submission (null before any).
  String? firstName = 'Jane';
  String? secondName = 'Wanjiru';
  bool hasPassword = true;

  /// The operating mode GET /v1/status reports (ADR 0027); null answers 404, as an api without one would.
  String? operatingMode;

  /// The web addresses the app opened in the browser (policy pages).
  final opened = <Uri>[];

  /// The Google account's photo (ADR 0035); and whether the sign-in is too old to set a first password.
  String? photoUrl;
  bool signInStale = false;

  /// Whether the "server" can email reset links; the addresses it was asked to email.
  bool resetsConfigured = true;
  final resetRequests = <String>[];

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
        path == '/v1/auth/providers' ||
        path == '/v1/status' ||
        path == '/v1/auth/password/forgot' ||
        path == '/v1/auth/password/reset' ||
        path.startsWith('/v1/auth/sso/') ||
        (method == 'GET' && path.startsWith('/v1/draws') && !path.endsWith('/mine') && path != '/v1/draws/periods');
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
        if (_identityMissing(body)) return _problem(400, 'invalid-identity', 'Check your names and ID number');
        if (body['email'] == null) {
          return _problem(400, 'invalid-email', 'Invalid email address');
        }
        if (body['email'] == 'taken@example.com') {
          return _problem(
            409,
            'email-registered',
            'This email address already has an account',
            'sign in instead, or use another address',
          );
        }
        email = body['email'] as String;
        signedIn = true;
        return _ok(tokens, 201);
      case ('POST', '/v1/auth/password/forgot'):
        if ((body['email'] as String? ?? '').isEmpty) {
          return _problem(400, 'invalid-email', 'An email address is required');
        }
        if (!resetsConfigured) {
          return _problem(503, 'unavailable', 'Password reset by email is not configured');
        }
        resetRequests.add(body['email'] as String);
        return http.Response('', 202);
      case ('POST', '/v1/auth/password/reset'):
        if (body['token'] != 'good-link') {
          return _problem(410, 'invalid-token', 'This link has expired or was already used', 'ask for a new one');
        }
        if ((body['new_password'] as String).length < 8) {
          return _problem(400, 'weak-password', 'Password too weak', 'too short');
        }
        return http.Response('', 204);
      case ('PUT', '/v1/account/email'):
        if (hasPassword && body['current_password'] != 'correct horse battery') {
          return _problem(403, 'wrong-password', 'Current password is incorrect');
        }
        if (body['email'] == 'taken@example.com') {
          return _problem(
            409,
            'email-registered',
            'This email address already has an account',
            'sign in instead, or use another address',
          );
        }
        email = body['email'] as String;
        return http.Response('', 204);
      case ('GET', '/v1/auth/providers'):
        return _ok(providers);
      case ('POST', '/v1/auth/sso/google') || ('POST', '/v1/auth/sso/apple'):
        if (body['nonce'] == null || body['id_token'] == null) {
          return _problem(400, 'invalid-body', 'Invalid body');
        }
        if (!ssoAccounts.contains(body['id_token'])) {
          return _problem(
            409,
            'sso-not-linked',
            'No account is linked to this identity',
            'give your phone number to create one',
          );
        }
        signedIn = true;
        return _ok(tokens);
      case ('POST', '/v1/auth/sso/google/signup') || ('POST', '/v1/auth/sso/apple/signup'):
        if (body['id_token'] == 'expired-token') {
          return _problem(401, 'invalid-token', 'Invalid token');
        }
        if (ssoAccounts.contains(body['id_token'])) {
          return _problem(409, 'sso-already-linked', 'This identity already signs in to an account');
        }
        if (_identityMissing(body)) return _problem(400, 'invalid-identity', 'Check your names and ID number');
        final digits = (body['msisdn'] as String? ?? '').replaceAll(' ', '');
        if (!RegExp(r'^(\+?254|0)[17]\d{8}$').hasMatch(digits)) {
          return _problem(
            400,
            'invalid-msisdn',
            'Invalid phone number',
            'use a Kenyan mobile number such as 0712345678',
          );
        }
        if (digits.endsWith('999999')) {
          return _problem(409, 'phone-registered', 'This number already has an account', 'sign in instead');
        }
        ssoAccounts.add(body['id_token'] as String);
        signedIn = true;
        return _ok(tokens, 201);
      case ('POST', '/v1/auth/refresh'):
        if (refreshFails) {
          return _problem(401, 'invalid-token', 'Invalid token');
        }
        return _ok(tokens);
      case ('POST', '/v1/auth/logout'):
        signedIn = false;
        return http.Response('', 204);
      case ('GET', '/v1/account'):
        return _ok({
          ...accountJson(
            kyc: kyc,
            phoneVerified: phoneVerified,
            restriction: restriction,
            closureRequestedAt: closureRequestedAt,
          ),
          'email': email,
          'has_password': hasPassword,
          'photo_url': photoUrl,
          'first_name': firstName,
          'second_name': secondName,
          'identity': {
            'status': identityStatus,
            'national_id_hint': identityStatus == 'none' ? null : (declared?['national_id'] as String?)?.substring(5),
          },
        });
      case ('POST', '/v1/account/identity'):
        if (_identityMissing(body)) return _problem(400, 'invalid-identity', 'Check your names and ID number');
        if (declareRefusal == 'kyc-state') {
          return _problem(
            409,
            'kyc-state',
            'Your identity is already submitted for verification',
            'the name on that submission is the one shown',
          );
        }
        if (declareRefusal != null || identityStatus != 'none') {
          return _problem(
            409,
            'identity-declared',
            'Your names and ID number are already on your account',
            'they are checked, not changed, before your first withdrawal; contact support if they are wrong',
          );
        }
        firstName = (body['first_name'] as String).trim();
        secondName = (body['second_name'] as String).trim();
        declared = {'first_name': firstName, 'second_name': secondName, 'national_id': body['national_id']};
        identityStatus = 'declared';
        return http.Response('', 204);
      case ('GET', '/v1/account/sessions'):
        return _ok({'sessions': sessions});
      case ('POST', '/v1/account/sessions/revoke-others'):
        sessions.removeWhere((s) => s['current'] != true);
        return http.Response('', 204);
      case ('POST', '/v1/auth/password/set'):
        if (hasPassword) {
          return _problem(409, 'password-exists', 'Your account has a password; change it with the current one');
        }
        if (signInStale) {
          return _problem(
            403,
            'sign-in-again',
            'Sign in again to set a password',
            'for your security, sign out, sign in with Google or Apple, then set it within 10 minutes',
          );
        }
        hasPassword = true;
        return http.Response('', 204);
      case ('GET', '/v1/account/sign-ins'):
        return _ok({'password': hasPassword, 'providers': linkedSignIns});
      case ('POST', '/v1/account/closure'):
        closureRequestedAt ??= '2026-10-26T12:00:00+03:00';
        return _ok({'closure_requested_at': closureRequestedAt}, 202);
      case ('GET', '/v1/kyc'):
        return _ok({'status': kyc, 'declared': declared});
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
          'top_up': {'min_kes': topUpMin, 'max_kes': 250000, 'wait_s': _topUpWaitS, 'cooldown_s': topUpCooldownS},
          'withdraw': withdrawBounds,
        });
      case ('GET', '/v1/payments'):
        for (final id in topUps.keys.toList()) {
          _settleClaim(id, count: false); // the list shows what the single reads settled
        }
        return _ok({'payments': topUps.values.toList().reversed.toList()});
      case ('POST', '/v1/payments/topups'):
        if (kyc != 'verified') {
          return _problem(403, 'kyc-required', 'Identity verification required');
        }
        final id = 'pay-${++_ids}';
        final amount = body['amount_kes']! as int;
        if (amount < topUpMin) {
          return _problem(400, 'invalid-amount', 'Invalid amount');
        }
        if (_topUpWaitS > 0) {
          return _problem(429, 'top-up-cooldown', 'Wait a moment before topping up again');
        }
        _lastTopUp = DateTime.now();
        topUps[id] = topUpJson(id: id, status: 'pending', amount: amount);
        _topUpPolls[id] = 0;
        return _ok(topUps[id]!, 202);
      case ('GET', '/v1/payouts'):
        return _ok({'payouts': payouts.values.toList().reversed.toList()});
      case ('POST', '/v1/payouts/withdrawals'):
        final amount = body['amount_kes']! as int;
        if (amount > balance) {
          return _problem(409, 'insufficient-funds', 'Your balance is below the amount');
        }
        balance -= amount;
        final id = 'po-${++_ids}';
        payouts[id] = payoutJson(id: id, amount: amount);
        return _ok(payouts[id]!, 201);
      case ('GET', '/v1/status'):
        if (operatingMode == null) return _problem(404, 'not-found', 'Not found');
        return _ok({
          'operating_mode': operatingMode,
          'real_money': operatingMode == 'live',
          'money_moves': operatingMode != 'disabled',
        });
      case ('GET', '/v1/draws'):
        return _ok({'draws': draws});
      case ('GET', '/v1/draws/periods'):
        return _ok({'periods': periods});
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
        // as the server does (ADR 0030): every session ends
        access = 'access-revoked';
        refreshFails = true;
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
    if (method == 'POST' && path.startsWith('/v1/payments/') && path.endsWith('/receipt')) {
      final id = path.split('/')[3];
      final t = topUps[id];
      if (t == null) {
        return _problem(404, 'not-found', 'Not found');
      }
      final receipt = (body['receipt']! as String).trim().toUpperCase();
      if (!RegExp(r'^[A-Z0-9]{10}$').hasMatch(receipt)) {
        return _problem(400, 'invalid-receipt', 'Check the receipt', 'an M-Pesa receipt is 10 letters and digits');
      }
      switch (t['status']) {
        case 'pending':
          return _problem(409, 'still-settling', 'This top-up is still being confirmed');
        case 'credited' || 'reversed':
          return _problem(409, 'not-claimable', 'This top-up is already credited');
      }
      final open = (t['receipt_claim'] as Map<String, Object?>?)?['status'];
      if (open == 'claimed' || open == 'checking') {
        return _problem(409, 'claim-open', 'A receipt is already being checked for this top-up');
      }
      topUps[id] = {
        ...t,
        'receipt_claim': {'receipt': receipt, 'status': 'claimed'},
      };
      _claimReads[id] = 0;
      return _ok({'receipt': receipt, 'status': 'claimed'}, 202);
    }
    if (method == 'GET' && path.startsWith('/v1/payments/')) {
      final id = path.split('/').last;
      final t = topUps[id];
      if (t == null) {
        return _problem(404, 'not-found', 'Not found');
      }
      _settleClaim(id);
      final polls = _topUpPolls[id] = _topUpPolls[id]! + 1;
      if (topUpPendingPolls != null && polls >= topUpPendingPolls! && t['status'] == 'pending') {
        final amount = t['amount_requested_kes']! as int;
        if (topUpOutcome == 'unconfirmed') {
          topUps[id] = topUpJson(id: id, status: 'unconfirmed', amount: amount);
        } else {
          topUps[id] = topUpJson(id: id, status: 'credited', amount: amount, confirmed: amount);
          balance += amount; // the operator pays the deposit excise
          phoneVerified = true;
        }
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
    'slots': 12,
    'lucky_slots': 1,
    'tickets': tickets,
  };

  int _chances() => (const [0, 1, 2, 3, 5][plays]) * (lucky ? 2 : 1);

  http.Response _play() {
    if (restriction != null) {
      return _problem(403, 'restricted', 'Your account is restricted');
    }
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
