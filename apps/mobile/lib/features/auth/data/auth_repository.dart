import '../../../core/api/api_client.dart';
import '../../../core/api/api_failure.dart';
import '../../../core/api/idempotency.dart';
import '../../../core/api/parse.dart';
import '../../../core/auth/session_manager.dart';
import '../../../core/platform/device_attestation.dart';
import 'provider_sign_in.dart';
import 'sign_in_providers.dart';

class AuthRepository {
  const AuthRepository(this._api, this._device, {this._attestation = const NoDeviceAttestation()});

  final ApiClient _api;
  final DeviceIdentity _device;
  final DeviceAttestation _attestation;

  Future<Tokens> signUp({
    required String msisdn,
    required String email,
    required String firstName,
    required String secondName,
    required String password,
    required String idempotencyKey,
  }) => _credentials('/v1/auth/signup', msisdn, password, idempotencyKey, email: email, names: (firstName, secondName));

  /// Asks for a password-reset link to be emailed. The API answers the same whether or not the address has an account.
  Future<void> forgotPassword({required String email, required String idempotencyKey}) => _api.post(
    '/v1/auth/password/forgot',
    idempotencyKey: idempotencyKey,
    authenticated: false,
    body: {'email': email},
  );

  /// Sets a new password from the token in an emailed link.
  Future<void> resetPassword({required String token, required String password, required String idempotencyKey}) =>
      _api.post(
        '/v1/auth/password/reset',
        idempotencyKey: idempotencyKey,
        authenticated: false,
        body: {'token': token, 'new_password': password},
      );

  Future<Tokens> logIn({required String msisdn, required String password, required String idempotencyKey}) =>
      _credentials('/v1/auth/login', msisdn, password, idempotencyKey);

  /// Which of Google and Apple the API offers (ADR 0020).
  Future<SignInProviders> providers() async {
    final r = await _api.get('/v1/auth/providers', authenticated: false);
    return parse(r, SignInProviders.fromJson);
  }

  /// Signs in with a Google or Apple token. An identity with no account yet fails with `sso-not-linked`: the screen
  /// then asks for a phone number and calls [signUpWithProvider] with the same token.
  Future<Tokens> signInWithProvider(ProviderToken t, {required String idempotencyKey}) =>
      _provider('/v1/auth/sso/${t.provider}', t, idempotencyKey);

  /// Creates the account for a Google or Apple identity from the player's phone number, and signs it in.
  Future<Tokens> signUpWithProvider(
    ProviderToken t, {
    required String msisdn,
    required String firstName,
    required String secondName,
    required String idempotencyKey,
  }) =>
      _provider('/v1/auth/sso/${t.provider}/signup', t, idempotencyKey, msisdn: msisdn, names: (firstName, secondName));

  Future<Tokens> _provider(String path, ProviderToken t, String key, {String? msisdn, (String, String)? names}) async {
    final r = await _api.post(
      path,
      idempotencyKey: key,
      authenticated: false,
      body: {
        'id_token': t.idToken,
        'nonce': t.nonce,
        'msisdn': ?msisdn,
        'first_name': ?names?.$1,
        'second_name': ?names?.$2,
        'device': {..._device.toJson(), ...await _evidence()},
      },
    );
    return parse(r, (j) => Tokens.fromJson(j.raw));
  }

  Future<Tokens> _credentials(
    String path,
    String msisdn,
    String password,
    String key, {
    String? email,
    (String, String)? names,
  }) async {
    final r = await _api.post(
      path,
      idempotencyKey: key,
      authenticated: false,
      body: {
        'msisdn': msisdn,
        'email': ?email,
        'first_name': ?names?.$1,
        'second_name': ?names?.$2,
        'password': password,
        'device': {..._device.toJson(), ...await _evidence()},
      },
    );
    return parse(r, (j) => Tokens.fromJson(j.raw));
  }

  /// A fresh server challenge and the platform's token for it, or nothing (the server then records the device as
  /// unattested; whether that matters is its decision).
  Future<Map<String, Object?>> _evidence() async {
    if (!_attestation.available) return const {};
    try {
      final r = await _api.post(
        '/v1/auth/attestation/challenge',
        idempotencyKey: newIdempotencyKey(),
        authenticated: false,
      );
      final challenge = parse(r, (j) => j.str('challenge'));
      final token = await _attestation.attest(challenge);
      return token == null ? const {} : {'attestation_token': token, 'attestation_challenge': challenge};
    } on ApiFailure {
      return const {};
    }
  }
}
