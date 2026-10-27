import '../../../core/api/api_client.dart';
import '../../../core/api/parse.dart';
import '../domain/draw_models.dart';

class DrawsRepository {
  const DrawsRepository(this._api);

  final ApiClient _api;

  Future<List<Draw>> list({int limit = 40}) async => parse(
    await _api.get('/v1/draws', query: {'limit': '$limit'}, authenticated: false),
    (j) => j.list('draws', Draw.fromJson),
  );

  Future<Draw> draw(String id) async => parse(await _api.get('/v1/draws/$id', authenticated: false), Draw.fromJson);

  Future<List<PlayerPeriod>> myPeriods() async =>
      parse(await _api.get('/v1/draws/periods'), (j) => j.list('periods', PlayerPeriod.fromJson));

  Future<MyDrawEntries> myEntries(String id) async =>
      parse(await _api.get('/v1/draws/$id/entries/mine'), MyDrawEntries.fromJson);
}
