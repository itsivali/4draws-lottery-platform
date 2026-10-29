import '../../../core/api/json.dart';

enum KycStatus { none, pending, verified, manualReview, rejected }

KycStatus kycStatusFrom(String s) => switch (s) {
  'none' => KycStatus.none,
  'pending' => KycStatus.pending,
  'verified' => KycStatus.verified,
  'manual_review' => KycStatus.manualReview,
  'rejected' => KycStatus.rejected,
  _ => throw FormatException('kyc status: $s'),
};

class Restriction {
  const Restriction({required this.kind, required this.until});

  factory Restriction.fromJson(JsonObject j) => Restriction(kind: j.str('kind'), until: j.instantOrNull('until'));

  /// `timeout` or `self_exclusion`.
  final String kind;

  /// Null for a permanent self-exclusion.
  final DateTime? until;

  bool get selfExclusion => kind == 'self_exclusion';
}

class Account {
  const Account({
    required this.userId,
    required this.msisdn,
    required this.phoneVerified,
    required this.kycStatus,
    required this.status,
    required this.createdAt,
    required this.restriction,
    required this.closureRequestedAt,
    this.mustChangePassword = false,
  });

  factory Account.fromJson(JsonObject j) => Account(
    mustChangePassword: j.raw['must_change_password'] == true,
    userId: j.str('user_id'),
    msisdn: j.str('msisdn'),
    phoneVerified: j.boolean('phone_verified'),
    kycStatus: kycStatusFrom(j.str('kyc_status')),
    status: j.str('status'),
    createdAt: j.instant('created_at'),
    restriction: switch (j.objectOrNull('restriction')) {
      final r? => Restriction.fromJson(r),
      null => null,
    },
    closureRequestedAt: j.instantOrNull('closure_requested_at'),
  );

  final String userId;

  /// 254XXXXXXXXX.
  final String msisdn;
  final bool phoneVerified;
  final KycStatus kycStatus;

  /// active, suspended or closed.
  final String status;
  final DateTime createdAt;
  final Restriction? restriction;
  final DateTime? closureRequestedAt;

  /// Staff gave the player a temporary password they have not replaced yet.
  final bool mustChangePassword;
}

class DepositLimit {
  const DepositLimit({required this.amountKes, required this.effectiveFrom});

  factory DepositLimit.fromJson(JsonObject j) =>
      DepositLimit(amountKes: j.integer('amount_kes'), effectiveFrom: j.instant('effective_from'));

  final int amountKes;
  final DateTime effectiveFrom;
}

class LimitState {
  const LimitState({this.current, this.pending});

  final DepositLimit? current;

  /// An increase waiting out its cooling-off.
  final DepositLimit? pending;
}

/// Deposit limits by period (`daily`, `weekly`, `monthly`).
Map<String, LimitState> limitsFromJson(JsonObject j) {
  final raw = j.object('limits').raw;
  return {
    for (final MapEntry(:key, :value) in raw.entries)
      if (value is Map<String, Object?>)
        key: LimitState(
          current: switch (JsonObject(value).objectOrNull('current')) {
            final c? => DepositLimit.fromJson(c),
            null => null,
          },
          pending: switch (JsonObject(value).objectOrNull('pending')) {
            final p? => DepositLimit.fromJson(p),
            null => null,
          },
        ),
  };
}
