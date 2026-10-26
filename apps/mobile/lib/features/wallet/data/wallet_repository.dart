import '../../../core/api/api_client.dart';
import '../../../core/api/parse.dart';
import '../domain/wallet_models.dart';

class WalletRepository {
  const WalletRepository(this._api);

  final ApiClient _api;

  Future<WalletStatement> statement({String? before}) async =>
      parse(await _api.get('/v1/wallet', query: {'limit': '20', 'before': ?before}), WalletStatement.fromJson);

  Future<List<TopUp>> topUps() async =>
      parse(await _api.get('/v1/payments', query: {'limit': '20'}), (j) => j.list('payments', TopUp.fromJson));

  /// Asks the API to send an M-Pesa prompt to the account's own number. The answer is `pending`: nothing is
  /// credited until the server confirms the payment with Daraja.
  Future<TopUp> startTopUp({required int amountKes, required String idempotencyKey}) async => parse(
    await _api.post('/v1/payments/topups', idempotencyKey: idempotencyKey, body: {'amount_kes': amountKes}),
    TopUp.fromJson,
  );

  Future<TopUp> topUp(String id) async => parse(await _api.get('/v1/payments/$id'), TopUp.fromJson);

  /// Debits the wallet and sends the money to the account's proved number; `processing` until M-Pesa's answer is
  /// confirmed.
  Future<Payout> withdraw({required int amountKes, required String idempotencyKey}) async => parse(
    await _api.post('/v1/payouts/withdrawals', idempotencyKey: idempotencyKey, body: {'amount_kes': amountKes}),
    Payout.fromJson,
  );

  Future<List<Payout>> payouts() async =>
      parse(await _api.get('/v1/payouts'), (j) => j.list('payouts', Payout.fromJson));

  Future<Payout> payout(String id) async => parse(await _api.get('/v1/payouts/$id'), Payout.fromJson);
}
