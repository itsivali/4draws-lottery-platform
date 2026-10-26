import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../app/providers.dart';
import '../data/play_repository.dart';
import '../domain/play_models.dart';

final playRepositoryProvider = Provider<PlayRepository>((ref) => PlayRepository(ref.watch(apiClientProvider)));

/// The player's play day as the server counts it.
final todayProvider = FutureProvider<Today>((ref) => ref.watch(playRepositoryProvider).today());
