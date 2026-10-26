import '../../../core/widgets/status_pill.dart';
import '../domain/draw_models.dart';

String drawTypeLabel(DrawType t) => switch (t) {
  DrawType.daily => 'Daily draw',
  DrawType.weekly => 'Weekly draw',
  DrawType.monthly => 'Monthly draw',
  DrawType.quarterly => 'Quarterly draw',
};

/// Each lifecycle state in words. "Results" only once winners are drawn; "final" only once approved.
(StatusTone, String) drawStatus(DrawStatus s) => switch (s) {
  DrawStatus.open => (StatusTone.neutral, 'Open · taking entries'),
  DrawStatus.frozen => (StatusTone.pending, 'Entries closed'),
  DrawStatus.signed => (StatusTone.pending, 'Signed · awaiting the draw'),
  DrawStatus.conducted => (StatusTone.pending, 'Drawn · awaiting approval'),
  DrawStatus.approved => (StatusTone.success, 'Results approved'),
  DrawStatus.paid => (StatusTone.success, 'Prizes paid'),
  DrawStatus.voided => (StatusTone.error, 'Voided · re-run as a new draw'),
};
