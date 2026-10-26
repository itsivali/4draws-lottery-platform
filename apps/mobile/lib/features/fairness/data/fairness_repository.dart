import '../../../core/api/api_client.dart';
import '../../../core/api/parse.dart';
import '../domain/fairness_models.dart';

class FairnessRepository {
  const FairnessRepository(this._api);

  final ApiClient _api;

  Future<Fairness> fairness() async => parse(await _api.get('/v1/fairness'), Fairness.fromJson);

  /// Reveals the active server seed and starts a new chain, optionally with the player's own client seed.
  Future<Fairness> rotate({required String idempotencyKey, String? clientSeed}) async => parse(
    await _api.post('/v1/fairness/rotate', idempotencyKey: idempotencyKey, body: {'client_seed': ?clientSeed}),
    Fairness.fromJson,
  );
}
