import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/app/router.dart';
import 'package:lottery_mobile/core/auth/session_manager.dart';

void main() {
  group('an address the app does not have, such as /login (owner, 2026-11-11)', () {
    test('the usual sign-in and sign-up addresses go to the real ones', () {
      for (final (from, to) in [
        ('/login', '/sign-in'),
        ('/LOGIN', '/sign-in'),
        ('/signin', '/sign-in'),
        ('/register', '/create-account'),
        ('/signup', '/create-account'),
      ]) {
        expect(sessionRedirect(SessionStatus.signedOut, Uri.parse(from)), to, reason: from);
        expect(sessionRedirect(SessionStatus.signedIn, Uri.parse(from)), to, reason: '$from, then home');
      }
    });

    test('signing in after a detour from an unknown address goes home, not to a missing page', () {
      expect(sessionRedirect(SessionStatus.signedIn, Uri.parse('/sign-in?from=%2Flogin')), '/home');
      expect(sessionRedirect(SessionStatus.signedIn, Uri.parse('/sign-in?from=%2Fnowhere%2Fat-all')), '/home');
      expect(sessionRedirect(SessionStatus.signedIn, Uri.parse('/sign-in?from=https%3A%2F%2Fevil.test')), '/home');
    });

    test('an unknown address is not remembered on the way to sign-in', () {
      expect(sessionRedirect(SessionStatus.signedOut, Uri.parse('/nowhere')), '/sign-in');
    });

    test('real pages still survive the detour', () {
      expect(sessionRedirect(SessionStatus.signedOut, Uri.parse('/wallet/top-up')), '/sign-in?from=%2Fwallet%2Ftop-up');
      expect(sessionRedirect(SessionStatus.signedIn, Uri.parse('/sign-in?from=%2Fdraws%2Fd1')), '/draws/d1');
    });
  });

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
