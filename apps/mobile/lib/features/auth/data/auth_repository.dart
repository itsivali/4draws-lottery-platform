import '../../../core/api/api_client.dart';
import '../../../core/api/api_failure.dart';
import '../../../core/api/idempotency.dart';
import '../../../core/api/parse.dart';
import '../../../core/auth/session_manager.dart';
import '../../../core/platform/device_attestation.dart';

class AuthRepository {
  const AuthRepository(this._api, this._device, {this._attestation = const NoDeviceAttestation()});

  final ApiClient _api;
  final DeviceIdentity _device;
  final DeviceAttestation _attestation;

  Future<Tokens> signUp({required String msisdn, required String password, required String idempotencyKey}) =>
      _credentials('/v1/auth/signup', msisdn, password, idempotencyKey);

  Future<Tokens> logIn({required String msisdn, required String password, required String idempotencyKey}) =>
      _credentials('/v1/auth/login', msisdn, password, idempotencyKey);

  Future<Tokens> _credentials(String path, String msisdn, String password, String key) async {
    final r = await _api.post(
      path,
      idempotencyKey: key,
      authenticated: false,
      body: {
        'msisdn': msisdn,
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
