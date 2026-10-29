import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import '../support/app_harness.dart';
import '../support/fake_backend.dart';
import '../support/fonts.dart';

/// Signing in with Google or Apple (ADR 0020): the buttons appear when the API offers them, an identity without an
/// account is asked for its M-Pesa number and nothing else, and one with an account goes straight in.
void main() {
  setUpAll(loadAppFonts);

  Future<void> tapGoogle(WidgetTester t) async {
    await t.tap(find.widgetWithText(OutlinedButton, 'Continue with Google'));
    await t.pumpAndSettle();
  }

  testWidgets('Google and Apple are offered beside the form, with the client ids the API gave', (t) async {
    final h = await pumpApp(t, FakeBackend(), signedIn: false);
    expect(find.text('Good to see you'), findsOneWidget);
    expect(find.text('OR CONTINUE WITH'), findsOneWidget);
    expect(find.text('Continue with Google'), findsOneWidget);
    expect(find.text('Continue with Apple'), findsOneWidget);
    expect(h.providerSignIn.prepared?.googleClientId, 'web-client.apps.googleusercontent.com');
  });

  testWidgets('without providers from the API, only the phone-and-password form is shown', (t) async {
    await pumpApp(t, FakeBackend()..providers = {}, signedIn: false);
    expect(find.text('Good to see you'), findsOneWidget);
    expect(find.text('Continue with Google'), findsNothing);
    expect(find.text('Continue with Apple'), findsNothing);
  });

  testWidgets('a Google account with no account yet is asked for a phone number, then lands on Home', (t) async {
    final backend = FakeBackend(balance: 1250);
    final h = await pumpApp(t, backend, signedIn: false);
    await tapGoogle(t);
    expect(h.providerSignIn.opened, 1);
    expect(find.text('Almost there'), findsOneWidget);
    expect(find.textContaining('Your Google account is ready'), findsOneWidget);
    expect(find.text('Password'), findsNothing, reason: 'no password is asked of a Google account');
    await t.enterText(find.widgetWithText(TextFormField, 'First name'), 'Jane');
    await t.enterText(find.widgetWithText(TextFormField, 'Second name'), 'Wanjiru');

    // the number is required, and so is the age confirmation
    await t.tap(find.widgetWithText(FilledButton, 'Create account'));
    await t.pumpAndSettle();
    expect(find.text('Enter a Kenyan mobile number such as 0712 345 678.'), findsOneWidget);
    await t.enterText(find.widgetWithText(TextFormField, 'M-Pesa phone number'), '0712 345 678');
    await t.tap(find.widgetWithText(FilledButton, 'Create account'));
    await t.pumpAndSettle();
    expect(find.text('Confirm that you are 18 or older to create an account.'), findsOneWidget);
    await t.tap(find.text('I am 18 or older'));
    await t.tap(find.widgetWithText(FilledButton, 'Create account'));
    await t.pumpAndSettle();

    expect(find.text('KES 1,250'), findsOneWidget);
    final signup = backend.requests.lastWhere((r) => r.url.path == '/v1/auth/sso/google/signup');
    expect(signup.body, contains('"id_token":"google-id-token"'));
    expect(signup.body, contains('"nonce":"n1"'));
    expect(signup.body, contains('"msisdn":"0712 345 678"'));
    expect(backend.ssoAccounts, contains('google-id-token'));
  });

  testWidgets('a Google account that already has an account signs straight in', (t) async {
    final backend = FakeBackend()..ssoAccounts.add('google-id-token');
    await pumpApp(t, backend, signedIn: false);
    await tapGoogle(t);
    expect(find.text('Almost there'), findsNothing);
    expect(find.text('Confirmed balance'), findsOneWidget);
    expect(backend.requests.where((r) => r.url.path.endsWith('/signup')), isEmpty);
  });

  testWidgets('Apple works the same way', (t) async {
    final h = await pumpApp(t, FakeBackend(), signedIn: false);
    h.providerSignIn.nextToken = 'apple-id-token';
    await t.tap(find.text('Continue with Apple'));
    await t.pumpAndSettle();
    expect(find.textContaining('Your Apple account is ready'), findsOneWidget);
  });

  testWidgets('a number with an account already is refused, with a way back to the form', (t) async {
    await pumpApp(t, FakeBackend(), signedIn: false);
    await tapGoogle(t);
    await t.enterText(find.widgetWithText(TextFormField, 'M-Pesa phone number'), '0712999999');
    await t.enterText(find.widgetWithText(TextFormField, 'First name'), 'Jane');
    await t.enterText(find.widgetWithText(TextFormField, 'Second name'), 'Wanjiru');
    await t.tap(find.text('I am 18 or older'));
    await t.tap(find.widgetWithText(FilledButton, 'Create account'));
    await t.pumpAndSettle();
    expect(find.text('This number already has an account'), findsOneWidget);
    expect(find.text('Sign in with that number and its password instead.'), findsOneWidget);
    await t.tap(find.text('Sign in another way'));
    await t.pumpAndSettle();
    expect(find.text('Good to see you'), findsOneWidget);
    expect(find.text('Continue with Google'), findsOneWidget);
  });

  testWidgets('a provider token that ran out sends the player back to start again', (t) async {
    final h = await pumpApp(t, FakeBackend(), signedIn: false);
    h.providerSignIn.nextToken = 'expired-token';
    await tapGoogle(t);
    await t.enterText(find.widgetWithText(TextFormField, 'M-Pesa phone number'), '0712345678');
    await t.enterText(find.widgetWithText(TextFormField, 'First name'), 'Jane');
    await t.enterText(find.widgetWithText(TextFormField, 'Second name'), 'Wanjiru');
    await t.tap(find.text('I am 18 or older'));
    await t.tap(find.widgetWithText(FilledButton, 'Create account'));
    await t.pumpAndSettle();
    expect(find.text('Good to see you'), findsOneWidget);
    expect(find.text('Your Google sign-in ran out. Start again.'), findsOneWidget);
  });
}
