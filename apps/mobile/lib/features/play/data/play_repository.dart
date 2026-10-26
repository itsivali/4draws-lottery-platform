import '../../../core/api/api_client.dart';
import '../../../core/api/parse.dart';
import '../domain/play_models.dart';

class PlayRepository {
  const PlayRepository(this._api);

  final ApiClient _api;

  /// Buys one play. The server charges, issues the ticket and spins in one transaction; replaying [idempotencyKey]
  /// returns the same play and charges nothing, which is how an uncertain play is reconciled.
  Future<PlayResult> play({required String idempotencyKey}) async {
    final r = await _api.post('/v1/plays', idempotencyKey: idempotencyKey);
    return parse(r, (j) => PlayResult.fromJson(j, replayed: r.status == 200 || r.replayed));
  }

  Future<Today> today() async => parse(await _api.get('/v1/plays/today'), Today.fromJson);
}
