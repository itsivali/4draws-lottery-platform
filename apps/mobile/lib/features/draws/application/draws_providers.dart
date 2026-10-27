import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../app/providers.dart';
import '../data/draws_repository.dart';
import '../domain/draw_models.dart';

final drawsRepositoryProvider = Provider<DrawsRepository>((ref) => DrawsRepository(ref.watch(apiClientProvider)));

final drawsProvider = FutureProvider<List<Draw>>((ref) => ref.watch(drawsRepositoryProvider).list());

final drawProvider = FutureProvider.family<Draw, String>((ref, id) => ref.watch(drawsRepositoryProvider).draw(id));

final myEntriesProvider = FutureProvider.family<MyDrawEntries, String>(
  (ref, id) => ref.watch(drawsRepositoryProvider).myEntries(id),
);

final myPeriodsProvider = FutureProvider<List<PlayerPeriod>>((ref) => ref.watch(drawsRepositoryProvider).myPeriods());
