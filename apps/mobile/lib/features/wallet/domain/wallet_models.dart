import '../../../core/api/json.dart';

class WalletTx {
  const WalletTx({
    required this.id,
    required this.kind,
    required this.amountKes,
    required this.referenceType,
    required this.createdAt,
  });

  factory WalletTx.fromJson(JsonObject j) => WalletTx(
    id: j.str('id'),
    kind: j.str('kind'),
    amountKes: j.integer('amount_kes'),
    referenceType: j.strOrNull('reference_type'),
    createdAt: j.instant('created_at'),
  );

  final String id;

  /// deposit, deposit_excise, stake, withdrawal, prize, refund, adjustment, reversal, debt_recovery.
  final String kind;

  /// Signed: credits positive, debits negative.
  final int amountKes;
  final String? referenceType;
  final DateTime createdAt;
}

/// The top-up range the server accepts. It is the server's to set (KES 1 while testing with real money), never the app's.
class TopUpBounds {
  const TopUpBounds({required this.minKes, required this.maxKes});

  factory TopUpBounds.fromJson(JsonObject j) => TopUpBounds(minKes: j.integer('min_kes'), maxKes: j.integer('max_kes'));

  /// Used until the wallet has loaded; the server checks every amount anyway.
  static const fallback = TopUpBounds(minKes: 10, maxKes: 250000);

  final int minKes;
  final int maxKes;

  /// The usual amounts, led by the minimum when it is below them.
  List<int> get quickPicks => [
    if (minKes < 100) minKes,
    ...[100, 200, 500, 1000].where((q) => q >= minKes && q <= maxKes),
  ];
}

class WalletStatement {
  const WalletStatement({
    required this.balanceKes,
    required this.debtKes,
    required this.transactions,
    this.nextBefore,
    this.topUp,
  });

  factory WalletStatement.fromJson(JsonObject j) => WalletStatement(
    balanceKes: j.integer('balance_kes'),
    debtKes: j.integer('debt_kes'),
    transactions: j.list('transactions', WalletTx.fromJson),
    nextBefore: j.strOrNull('next_before'),
    topUp: switch (j.objectOrNull('top_up')) {
      final b? => TopUpBounds.fromJson(b),
      null => null,
    },
  );

  /// Null when the server cannot take top-ups right now.
  final TopUpBounds? topUp;

  final int balanceKes;

  /// Owed after a reversed top-up; play and withdrawals are paused while it is above 0.
  final int debtKes;
  final List<WalletTx> transactions;
  final String? nextBefore;
}

enum TopUpStatus {
  pending,
  credited,
  failed,
  reversed;

  bool get settled => this != pending;
}

class TopUp {
  const TopUp({
    required this.id,
    required this.status,
    required this.amountRequestedKes,
    required this.amountConfirmedKes,
    required this.createdAt,
  });

  factory TopUp.fromJson(JsonObject j) => TopUp(
    id: j.str('id'),
    status: TopUpStatus.values.byName(j.str('status')),
    amountRequestedKes: j.integer('amount_requested_kes'),
    amountConfirmedKes: j.intOrNull('amount_confirmed_kes'),
    createdAt: j.instant('created_at'),
  );

  final String id;
  final TopUpStatus status;
  final int amountRequestedKes;

  /// What M-Pesa confirmed the player paid (gross, before excise); null until credited.
  final int? amountConfirmedKes;
  final DateTime createdAt;
}

enum PayoutStatus {
  processing,
  paid,
  failed;

  bool get settled => this != processing;
}

class Payout {
  const Payout({
    required this.id,
    required this.kind,
    required this.status,
    required this.amountKes,
    required this.taxKes,
    required this.debtRepaidKes,
    required this.netKes,
    required this.receipt,
    required this.createdAt,
  });

  factory Payout.fromJson(JsonObject j) => Payout(
    id: j.str('id'),
    kind: j.str('kind'),
    status: PayoutStatus.values.byName(j.str('status')),
    amountKes: j.integer('amount_kes'),
    taxKes: j.integer('tax_kes'),
    debtRepaidKes: j.integer('debt_repaid_kes'),
    netKes: j.integer('net_kes'),
    receipt: j.strOrNull('receipt'),
    createdAt: j.instant('created_at'),
  );

  final String id;

  /// `withdrawal` or `prize`.
  final String kind;
  final PayoutStatus status;
  final int amountKes;
  final int taxKes;
  final int debtRepaidKes;

  /// What M-Pesa pays the player.
  final int netKes;
  final String? receipt;
  final DateTime createdAt;
}
