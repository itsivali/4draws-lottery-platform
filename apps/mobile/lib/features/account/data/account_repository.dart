import '../../../core/api/api_client.dart';
import '../../../core/api/parse.dart';
import '../domain/account_models.dart';

class AccountRepository {
  const AccountRepository(this._api);

  final ApiClient _api;

  Future<Account> account() async => parse(await _api.get('/v1/account'), Account.fromJson);

  Future<KycStatus> kyc() async => parse(await _api.get('/v1/kyc'), (j) => kycStatusFrom(j.str('status')));

  Future<KycStatus> submitKyc({
    required String nationalId,
    required String fullName,
    required DateTime dateOfBirth,
    required String idempotencyKey,
  }) async {
    final dob =
        '${dateOfBirth.year.toString().padLeft(4, '0')}-${dateOfBirth.month.toString().padLeft(2, '0')}-'
        '${dateOfBirth.day.toString().padLeft(2, '0')}';
    final r = await _api.post(
      '/v1/kyc',
      idempotencyKey: idempotencyKey,
      body: {'national_id': nationalId, 'full_name': fullName, 'date_of_birth': dob},
    );
    return parse(r, (j) => kycStatusFrom(j.str('status')));
  }

  Future<Map<String, LimitState>> limits() async => parse(await _api.get('/v1/rg/limits'), limitsFromJson);

  Future<void> setLimit({required String period, required int amountKes, required String idempotencyKey}) =>
      _api.put('/v1/rg/limits', idempotencyKey: idempotencyKey, body: {'period': period, 'amount_kes': amountKes});

  Future<void> startTimeout({required String duration, required String idempotencyKey}) =>
      _api.post('/v1/rg/timeout', idempotencyKey: idempotencyKey, body: {'duration': duration});

  Future<void> selfExclude({required String term, required String idempotencyKey}) =>
      _api.post('/v1/rg/self-exclusion', idempotencyKey: idempotencyKey, body: {'term': term});

  Future<void> changePassword({required String current, required String next, required String idempotencyKey}) =>
      _api.post(
        '/v1/auth/password/change',
        idempotencyKey: idempotencyKey,
        body: {'current_password': current, 'new_password': next},
      );

  /// Changes or adds the email address; an account with a password gives it.
  Future<void> setEmail({required String email, required String currentPassword, required String idempotencyKey}) =>
      _api.put(
        '/v1/account/email',
        idempotencyKey: idempotencyKey,
        body: {'email': email, 'current_password': currentPassword},
      );

  Future<DateTime> requestClosure({required String idempotencyKey}) async => parse(
    await _api.post('/v1/account/closure', idempotencyKey: idempotencyKey),
    (j) => j.instant('closure_requested_at'),
  );
}
