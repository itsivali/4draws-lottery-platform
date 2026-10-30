import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:lottery_mobile/core/api/api_client.dart';
import 'package:lottery_mobile/core/auth/session_manager.dart';
import 'package:lottery_mobile/core/platform/device_attestation.dart';
import 'package:lottery_mobile/features/auth/data/auth_repository.dart';

class NoTokens implements AccessTokenSource {
  @override
  Future<String?> accessToken() async => null;
  @override
  Future<String?> refreshAfterRejection(String rejected) async => null;
}

class FakeAttestation implements DeviceAttestation {
  final challenges = <String>[];
  @override
  bool get available => true;
  @override
  Future<String?> attest(String challenge) async {
    challenges.add(challenge);
    return 'token-for-$challenge';
  }
}

final tokens = {'access_token': 'a', 'refresh_token': 'r', 'expires_at': '2026-10-26T12:00:00Z'};

void main() {
  test('with an attestor, sign-in asks for a challenge and sends the token made for it', () async {
    final seen = <http.Request>[];
    final api = ApiClient(
      baseUrl: Uri.parse('https://api.test'),
      http: MockClient((r) async {
        seen.add(r);
        if (r.url.path == '/v1/auth/attestation/challenge') {
          return http.Response(jsonEncode({'challenge': 'c-123', 'expires_at': '2026-10-26T12:05:00Z'}), 201);
        }
        return http.Response(jsonEncode(tokens), 200);
      }),
      tokens: NoTokens(),
    );
    final att = FakeAttestation();
    final repo = AuthRepository(
      api,
      const DeviceIdentity(platform: 'android', fingerprint: 'fp'),
      attestation: att,
    );
    await repo.logIn(msisdn: '0712345678', password: 'pw-pw-pw-pw', idempotencyKey: 'k');
    expect(att.challenges, ['c-123']);
    final device = (jsonDecode(seen.last.body) as Map<String, Object?>)['device']! as Map<String, Object?>;
    expect(device['attestation_token'], 'token-for-c-123');
    expect(device['attestation_challenge'], 'c-123');
  });

  test('without an attestor (the web, or no native support yet) nothing extra is sent', () async {
    final seen = <http.Request>[];
    final api = ApiClient(
      baseUrl: Uri.parse('https://api.test'),
      http: MockClient((r) async {
        seen.add(r);
        return http.Response(jsonEncode(tokens), 201);
      }),
      tokens: NoTokens(),
    );
    final repo = AuthRepository(api, const DeviceIdentity(platform: 'web'));
    await repo.signUp(
      msisdn: '0712345678',
      email: 'jane@example.com',
      password: 'pw-pw-pw-pw',
      identity: const SignUpIdentity(firstName: ' Jane ', secondName: 'Wanjiru', nationalId: '12345678'),
      idempotencyKey: 'k',
    );
    expect(seen, hasLength(1));
    final sent = jsonDecode(seen.single.body) as Map<String, Object?>;
    // the names and ID number go with the sign-up, trimmed (owner, 2026-10-30)
    expect([sent['first_name'], sent['second_name'], sent['national_id']], ['Jane', 'Wanjiru', '12345678']);
    final device = sent['device']! as Map<String, Object?>;
    expect(device.containsKey('attestation_token'), isFalse);
  });

  test(
    'if attestation fails the sign-in still goes ahead, without a token (the server decides what that means)',
    () async {
      final api = ApiClient(
        baseUrl: Uri.parse('https://api.test'),
        http: MockClient(
          (r) async => r.url.path.endsWith('/challenge')
              ? http.Response(
                  '{"type":"https://lottery.example/problems/attestation-unavailable","title":"x","status":503}',
                  503,
                )
              : http.Response(jsonEncode(tokens), 200),
        ),
        tokens: NoTokens(),
      );
      final repo = AuthRepository(
        api,
        const DeviceIdentity(platform: 'ios', fingerprint: 'fp'),
        attestation: FakeAttestation(),
      );
      final t = await repo.logIn(msisdn: '0712345678', password: 'pw-pw-pw-pw', idempotencyKey: 'k');
      expect(t.access, 'a');
    },
  );
}
