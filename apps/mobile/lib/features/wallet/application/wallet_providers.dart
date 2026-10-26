import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../app/providers.dart';
import '../data/wallet_repository.dart';
import '../domain/wallet_models.dart';

final walletRepositoryProvider = Provider<WalletRepository>((ref) => WalletRepository(ref.watch(apiClientProvider)));

/// The confirmed balance, debt and latest transactions.
final walletStatementProvider = FutureProvider<WalletStatement>(
  (ref) => ref.watch(walletRepositoryProvider).statement(),
);

final topUpsProvider = FutureProvider<List<TopUp>>((ref) => ref.watch(walletRepositoryProvider).topUps());

final payoutsProvider = FutureProvider<List<Payout>>((ref) => ref.watch(walletRepositoryProvider).payouts());
