import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/app/router.dart';
import 'package:lottery_mobile/core/auth/session_manager.dart';

void main() {
  group('the create-account address (2026-10-30)', () {
    test('a signed-out player may be there, and is not sent to sign-in', () {
      expect(sessionRedirect(SessionStatus.signedOut, Uri.parse('/create-account')), isNull);
    });

    test('a signed-in player is sent home from it, as from sign-in', () {
      expect(sessionRedirect(SessionStatus.signedIn, Uri.parse('/create-account')), '/home');
      expect(sessionRedirect(SessionStatus.signedIn, Uri.parse('/create-account?from=%2Fwallet')), '/wallet');
    });
  });

  group('a temporary password holds the player on the password screen', () {
    test('every destination becomes the required password screen', () {
      for (final where in ['/home', '/play', '/wallet/top-up', '/account', '/']) {
        expect(
          sessionRedirect(SessionStatus.signedIn, Uri.parse(where), mustChangePassword: true),
          requiredPasswordRoute,
          reason: where,
        );
      }
    });

    test('the password screen itself is allowed', () {
      expect(
        sessionRedirect(SessionStatus.signedIn, Uri.parse('/account/password?required=1'), mustChangePassword: true),
        isNull,
      );
    });

    test('without the flag nothing changes', () {
      expect(sessionRedirect(SessionStatus.signedIn, Uri.parse('/play')), isNull);
      expect(sessionRedirect(SessionStatus.signedIn, Uri.parse('/')), '/home');
    });

    test('a signed-out player goes to sign-in whatever the flag says', () {
      expect(
        sessionRedirect(SessionStatus.signedOut, Uri.parse('/home'), mustChangePassword: true),
        startsWith('/sign-in'),
      );
    });
  });

  test('Tokens read the temporary-password flag, absent meaning false', () {
    final base = {'access_token': 'a', 'refresh_token': 'r', 'expires_at': '2026-10-29T12:00:00Z'};
    expect(Tokens.fromJson(base).mustChangePassword, isFalse);
    expect(Tokens.fromJson({...base, 'must_change_password': true}).mustChangePassword, isTrue);
  });
}
