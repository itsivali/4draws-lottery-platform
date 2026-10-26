import '../../../core/api/api_client.dart';
import '../../../core/api/parse.dart';
import '../../../core/auth/session_manager.dart';

class AuthRepository {
  const AuthRepository(this._api, this._device);

  final ApiClient _api;
  final DeviceIdentity _device;

  Future<Tokens> signUp({required String msisdn, required String password, required String idempotencyKey}) =>
      _credentials('/v1/auth/signup', msisdn, password, idempotencyKey);

  Future<Tokens> logIn({required String msisdn, required String password, required String idempotencyKey}) =>
      _credentials('/v1/auth/login', msisdn, password, idempotencyKey);

  Future<Tokens> _credentials(String path, String msisdn, String password, String key) async {
    final r = await _api.post(
      path,
      idempotencyKey: key,
      authenticated: false,
      body: {'msisdn': msisdn, 'password': password, 'device': _device.toJson()},
    );
    return parse(r, (j) => Tokens.fromJson(j.raw));
  }
}
