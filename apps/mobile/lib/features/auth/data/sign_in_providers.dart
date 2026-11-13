import 'package:flutter/foundation.dart';

import '../../../core/api/json.dart';

/// What the API offers for signing in with Google or Apple (`GET /v1/auth/providers`, ADR 0020): the client ids the
/// apps request ID tokens for. Nothing here is secret, and nothing here decides who may sign in: the API does.
class SignInProviders {
  const SignInProviders({this.googleClientId, this.googleIosClientId, this.appleClientId, this.apple = false});

  /// Sign in with Apple is shelved (owner, 2026-11-13): the apps do not offer it, whatever the API answers. The code
  /// behind it stays for the day it returns; turn this off then.
  static const appleShelved = true;

  factory SignInProviders.fromJson(JsonObject j) {
    final google = j.objectOrNull('google');
    final apple = appleShelved ? null : j.objectOrNull('apple');
    return SignInProviders(
      googleClientId: google?.str('client_id'),
      googleIosClientId: google?.strOrNull('ios_client_id'),
      appleClientId: apple?.strOrNull('client_id'),
      apple: apple != null,
    );
  }

  /// Neither is offered (also what the app assumes when it cannot ask).
  static const none = SignInProviders();

  /// The web app's OAuth client, which the phone apps name as their server client; null: Google is not offered.
  final String? googleClientId;

  /// The iOS app's own OAuth client, when it has one.
  final String? googleIosClientId;

  /// The Services ID the web app and Android use for Sign in with Apple; null with [apple] true: iOS only.
  final String? appleClientId;
  final bool apple;

  bool get google => googleClientId != null;

  /// Whether Apple can be offered on this platform: natively on iOS, elsewhere only through a Services ID.
  bool get appleHere => apple && (appleClientId != null || (!kIsWeb && defaultTargetPlatform == TargetPlatform.iOS));

  bool get any => google || appleHere;
}
