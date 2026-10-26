import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../app/providers.dart';
import '../data/account_repository.dart';
import '../domain/account_models.dart';

final accountRepositoryProvider = Provider<AccountRepository>((ref) => AccountRepository(ref.watch(apiClientProvider)));

final accountProvider = FutureProvider<Account>((ref) => ref.watch(accountRepositoryProvider).account());

final limitsProvider = FutureProvider<Map<String, LimitState>>((ref) => ref.watch(accountRepositoryProvider).limits());
