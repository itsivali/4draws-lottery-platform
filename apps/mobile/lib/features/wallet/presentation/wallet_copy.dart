import '../../../core/widgets/status_pill.dart';
import '../domain/wallet_models.dart';

String txLabel(String kind) => switch (kind) {
  'deposit' => 'M-Pesa top-up',
  'deposit_excise' => 'Excise duty on top-up',
  'stake' => 'Play',
  'withdrawal' => 'Withdrawal to M-Pesa',
  'prize' => 'Prize',
  'refund' => 'Refund',
  'adjustment' => 'Adjustment',
  'reversal' => 'Top-up reversed by M-Pesa',
  'debt_recovery' => 'Repayment of reversed top-up',
  _ => kind,
};

(StatusTone, String) topUpStatus(TopUpStatus s) => switch (s) {
  TopUpStatus.pending => (StatusTone.pending, 'Pending M-Pesa confirmation'),
  TopUpStatus.credited => (StatusTone.success, 'Credited'),
  TopUpStatus.failed => (StatusTone.error, 'Not completed'),
  TopUpStatus.unconfirmed => (StatusTone.warning, 'Not confirmed yet · being checked'),
  TopUpStatus.reversed => (StatusTone.warning, 'Reversed by M-Pesa'),
};

/// A line under an uncredited top-up about the receipt the player gave, or may give.
String? receiptClaimLine(TopUp t) => switch (t.claim?.status) {
  ReceiptClaimStatus.claimed || ReceiptClaimStatus.checking => 'Receipt ${t.claim!.receipt} sent · being confirmed',
  ReceiptClaimStatus.unverified => 'Receipt ${t.claim!.receipt} not confirmed yet · our team is checking',
  ReceiptClaimStatus.rejected => 'Receipt ${t.claim!.receipt} not accepted',
  ReceiptClaimStatus.confirmed => null,
  null => t.status == TopUpStatus.unconfirmed ? 'Got an M-Pesa message? Tap to enter the receipt' : null,
};

(StatusTone, String) payoutStatus(PayoutStatus s) => switch (s) {
  PayoutStatus.processing => (StatusTone.pending, 'Processing'),
  PayoutStatus.paid => (StatusTone.success, 'Paid'),
  PayoutStatus.failed => (StatusTone.error, 'Failed · money returned to wallet'),
};
