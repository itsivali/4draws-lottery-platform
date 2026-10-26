import '../../account/domain/account_models.dart';
import '../../wallet/domain/wallet_models.dart';
import 'play_models.dart';

enum PlayBlockReason {
  suspended,
  closed,
  selfExcluded,
  timedOut,
  kycNeeded,
  kycInReview,
  kycRejected,
  phoneNotProved,
  inDebt,
  dailyLimit,
  lowBalance,
}

class PlayBlock {
  const PlayBlock(this.reason, {this.until});

  final PlayBlockReason reason;

  /// When a time-out ends (null for anything else, and for a permanent self-exclusion).
  final DateTime? until;
}

/// Why the player cannot play right now, from the server's own state, or null when nothing in it stands in the way.
/// The server still decides: this only keeps the button honest and explains why it is off. Reasons are checked from
/// the most fundamental to the most practical, so a self-excluded player is never told to top up.
PlayBlock? playBlock({required Account account, required Today today, required WalletStatement wallet}) {
  if (account.status == 'closed') return const PlayBlock(PlayBlockReason.closed);
  if (account.status != 'active') return const PlayBlock(PlayBlockReason.suspended);
  final r = account.restriction;
  if (r != null) {
    return r.selfExclusion
        ? PlayBlock(PlayBlockReason.selfExcluded, until: r.until)
        : PlayBlock(PlayBlockReason.timedOut, until: r.until);
  }
  switch (account.kycStatus) {
    case KycStatus.none:
      return const PlayBlock(PlayBlockReason.kycNeeded);
    case KycStatus.pending || KycStatus.manualReview:
      return const PlayBlock(PlayBlockReason.kycInReview);
    case KycStatus.rejected:
      return const PlayBlock(PlayBlockReason.kycRejected);
    case KycStatus.verified:
      break;
  }
  if (!account.phoneVerified) return const PlayBlock(PlayBlockReason.phoneNotProved);
  if (wallet.debtKes > 0) return const PlayBlock(PlayBlockReason.inDebt);
  if (today.playsLeft <= 0) return const PlayBlock(PlayBlockReason.dailyLimit);
  if (wallet.balanceKes < today.priceKes) return const PlayBlock(PlayBlockReason.lowBalance);
  return null;
}
