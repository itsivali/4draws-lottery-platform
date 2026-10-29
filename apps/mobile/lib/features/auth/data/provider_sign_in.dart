import 'dart:async';
import 'dart:convert';
import 'dart:math';

import 'package:crypto/crypto.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';
import 'package:google_sign_in/google_sign_in.dart';
import 'package:sign_in_with_apple/sign_in_with_apple.dart';

import 'google_button_stub.dart' if (dart.library.js_interop) 'google_button_web.dart' as web;
import 'sign_in_providers.dart';

/// An ID token from Google or Apple, bound to the nonce it was requested with. The API checks both (ADR 0020).
class ProviderToken {
  const ProviderToken({required this.provider, required this.idToken, required this.nonce});

  final String provider; // google | apple
  final String idToken;
  final String nonce;
}

/// Gets ID tokens from the providers' own sign-in on this platform. The sign-in screen only ever sees tokens.
abstract interface class ProviderSignIn {
  /// Readies the SDKs for what the API offers; called once before any button is shown.
  Future<void> prepare(SignInProviders providers);

  /// Google's own button where its sign-in insists on rendering it (the web); null where the app draws the button.
  Widget? googleButton();

  /// Every token obtained: from [google] and [apple], and from the button on the web.
  Stream<ProviderToken> get tokens;

  /// Opens Google. Returns when the window closed; the token, if any, arrives on [tokens].
  Future<void> google();

  /// Opens Apple (natively on iOS, in the browser elsewhere). The token, if any, arrives on [tokens].
  Future<void> apple(SignInProviders providers);
}

/// Never offers anything (tests, and platforms without the SDKs).
class NoProviderSignIn implements ProviderSignIn {
  const NoProviderSignIn();

  @override
  Future<void> prepare(SignInProviders providers) async {}
  @override
  Widget? googleButton() => null;
  @override
  Stream<ProviderToken> get tokens => const Stream.empty();
  @override
  Future<void> google() async {}
  @override
  Future<void> apple(SignInProviders providers) async {}
}

String randomNonce() {
  final r = Random.secure();
  return List.generate(16, (_) => r.nextInt(256).toRadixString(16).padLeft(2, '0')).join();
}

String sha256Hex(String s) => sha256.convert(utf8.encode(s)).toString();

/// The real thing: the google_sign_in and sign_in_with_apple plugins. One Google nonce per app run (the SDK is
/// initialised once); a fresh Apple nonce per attempt. Apple receives the SHA-256 of the nonce and puts that in the
/// token, as it requires; the API hashes the nonce it is given before comparing.
class SdkProviderSignIn implements ProviderSignIn {
  SdkProviderSignIn({required this.apiBaseUrl});

  /// Where Apple sends the Android app's sign-in back to (`/auth/apple/android`, which bounces into the app).
  final Uri apiBaseUrl;

  final _tokens = StreamController<ProviderToken>.broadcast();
  Future<void>? _prepared;
  bool _googleReady = false;

  @override
  Stream<ProviderToken> get tokens => _tokens.stream;

  @override
  Future<void> prepare(SignInProviders providers) => _prepared ??= _prepare(providers);

  Future<void> _prepare(SignInProviders p) async {
    if (!p.google) return;
    final ios = !kIsWeb && defaultTargetPlatform == TargetPlatform.iOS;
    final nonce = randomNonce();
    final g = GoogleSignIn.instance;
    await g.initialize(
      // the web client signs the web app in directly; the phone apps name it as the server the token is for
      clientId: kIsWeb ? p.googleClientId : (ios ? p.googleIosClientId : null),
      serverClientId: kIsWeb ? null : p.googleClientId,
      nonce: nonce,
    );
    _googleReady = true;
    g.authenticationEvents.listen((e) {
      if (e is GoogleSignInAuthenticationEventSignIn) {
        final t = e.user.authentication.idToken;
        if (t != null) _tokens.add(ProviderToken(provider: 'google', idToken: t, nonce: nonce));
      }
    }, onError: (_) {});
  }

  @override
  Widget? googleButton() => kIsWeb && _googleReady ? web.googleButton() : null;

  @override
  Future<void> google() async {
    if (!_googleReady) return;
    try {
      await GoogleSignIn.instance.authenticate(); // the token arrives through authenticationEvents
    } on GoogleSignInException catch (e) {
      if (e.code == GoogleSignInExceptionCode.canceled || e.code == GoogleSignInExceptionCode.interrupted) return;
      rethrow;
    }
  }

  @override
  Future<void> apple(SignInProviders p) async {
    final nonce = randomNonce();
    final serviceId = p.appleClientId;
    final AuthorizationCredentialAppleID c;
    try {
      c = await SignInWithApple.getAppleIDCredential(
        scopes: const [AppleIDAuthorizationScopes.email],
        nonce: sha256Hex(nonce),
        webAuthenticationOptions: serviceId == null
            ? null
            : WebAuthenticationOptions(clientId: serviceId, redirectUri: kIsWeb ? _webReturn : _androidReturn),
      );
    } on SignInWithAppleAuthorizationException catch (e) {
      if (e.code == AuthorizationErrorCode.canceled) return;
      rethrow;
    }
    final t = c.identityToken;
    if (t == null) throw StateError('Apple returned no identity token');
    _tokens.add(ProviderToken(provider: 'apple', idToken: t, nonce: nonce));
  }

  /// The web app's own address, as registered with Apple as a return URL (Apple's popup posts back to the opener).
  Uri get _webReturn => Uri(scheme: Uri.base.scheme, host: Uri.base.host, port: Uri.base.port, path: '/');

  Uri get _androidReturn {
    final base = apiBaseUrl.path.endsWith('/')
        ? apiBaseUrl.path.substring(0, apiBaseUrl.path.length - 1)
        : apiBaseUrl.path;
    return apiBaseUrl.replace(path: '$base/auth/apple/android');
  }
}
