import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../app/providers.dart';
import '../data/fairness_repository.dart';
import '../domain/fairness_models.dart';

final fairnessRepositoryProvider = Provider<FairnessRepository>(
  (ref) => FairnessRepository(ref.watch(apiClientProvider)),
);

final fairnessProvider = FutureProvider<Fairness>((ref) => ref.watch(fairnessRepositoryProvider).fairness());
