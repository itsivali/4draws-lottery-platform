import '../../../core/api/api_client.dart';
import '../../../core/api/parse.dart';
import '../domain/account_models.dart';

class AccountRepository {
  const AccountRepository(this._api);

  final ApiClient _api;

  Future<Account> account() async => parse(await _api.get('/v1/account'), Account.fromJson);

  Future<KycStatus> kyc() async => parse(await _api.get('/v1/kyc'), (j) => kycStatusFrom(j.str('status')));

  /// The names and ID number the player gave at sign-up, to start the verification from (null for older accounts).
  Future<DeclaredIdentity?> declaredIdentity() async => parse(
    await _api.get('/v1/kyc'),
    (j) => switch (j.objectOrNull('declared')) {
      final d? => DeclaredIdentity(
        firstName: d.str('first_name'),
        secondName: d.str('second_name'),
        nationalId: d.str('national_id'),
      ),
      null => null,
    },
  );

  /// Submits the ID for verification. No date of birth is asked (owner, 2026-10-30): the M-Pesa line is registered to
  /// an adult against their ID, so staff check the name against the ID number.
  Future<KycStatus> submitKyc({
    required String nationalId,
    required String fullName,
    required String idempotencyKey,
  }) async {
    final r = await _api.post(
      '/v1/kyc',
      idempotencyKey: idempotencyKey,
      body: {'national_id': nationalId, 'full_name': fullName},
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

  /// Sets a first password on an account that signs in with Google or Apple only (ADR 0035): allowed within ten
  /// minutes of signing in.
  Future<void> setPassword({required String next, required String idempotencyKey}) =>
      _api.post('/v1/auth/password/set', idempotencyKey: idempotencyKey, body: {'new_password': next});

  /// Changes or adds the email address; an account with a password gives it.
  Future<void> setEmail({required String email, required String currentPassword, required String idempotencyKey}) =>
      _api.put(
        '/v1/account/email',
        idempotencyKey: idempotencyKey,
        body: {'email': email, 'current_password': currentPassword},
      );

  /// Gives the names and ID number of an account that gave none at sign-up (owner, 2026-10-31): once, and only until
  /// an identity submission exists (409 `identity-declared` / `kyc-state`).
  Future<void> declareIdentity({
    required String firstName,
    required String secondName,
    required String nationalId,
    required String idempotencyKey,
  }) => _api.post(
    '/v1/account/identity',
    idempotencyKey: idempotencyKey,
    body: {'first_name': firstName, 'second_name': secondName, 'national_id': nationalId},
  );

  /// The devices signed in to the account, newest first, this one marked.
  Future<List<DeviceSession>> sessions() async =>
      parse(await _api.get('/v1/account/sessions'), (j) => j.list('sessions', DeviceSession.fromJson));

  /// Signs every other device out; this one stays signed in.
  Future<void> signOutOtherDevices({required String idempotencyKey}) =>
      _api.post('/v1/account/sessions/revoke-others', idempotencyKey: idempotencyKey);

  Future<SignInMethods> signIns() async => parse(await _api.get('/v1/account/sign-ins'), SignInMethods.fromJson);

  Future<DateTime> requestClosure({required String idempotencyKey}) async => parse(
    await _api.post('/v1/account/closure', idempotencyKey: idempotencyKey),
    (j) => j.instant('closure_requested_at'),
  );
}
