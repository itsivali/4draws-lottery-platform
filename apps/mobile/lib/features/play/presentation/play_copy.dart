import '../../../core/formatting/dates.dart';
import '../../../core/formatting/money.dart';
import '../domain/play_eligibility.dart';
import '../domain/play_models.dart';

/// Why the play button is off, in plain words, and the one action that helps (null when nothing in the app does).
/// `dailyCap` is today's plays plus the plays left, as the server counts them.
({String title, String body, String? action, String? route}) describeBlock(
  PlayBlock b, {
  required int priceKes,
  required int dailyCap,
}) => switch (b.reason) {
  PlayBlockReason.suspended => (
    title: 'Your account is suspended',
    body: 'Play is not available. Please contact support.',
    action: null,
    route: null,
  ),
  PlayBlockReason.closed => (
    title: 'This account is closed',
    body: 'Play is not available.',
    action: null,
    route: null,
  ),
  PlayBlockReason.selfExcluded => (
    title: "You're self-excluded",
    body: b.until == null
        ? 'You chose a permanent self-exclusion. Play is closed.'
        : 'Play is closed until ${formatEat(b.until!)}.',
    action: null,
    route: null,
  ),
  PlayBlockReason.timedOut => (
    title: "You're taking a break",
    body: b.until == null ? 'Play is paused for now.' : 'Play resumes at ${formatEat(b.until!)}.',
    action: null,
    route: null,
  ),
  PlayBlockReason.kycNeeded => (
    title: 'Verify your identity to play',
    body: 'We check your national ID (and that you are 18 or older) before your first play.',
    action: 'Verify my ID',
    route: '/account/verify',
  ),
  PlayBlockReason.kycInReview => (
    title: 'Your ID is being checked',
    body: 'Staff review each ID by hand. You can play once it is approved.',
    action: null,
    route: null,
  ),
  PlayBlockReason.kycRejected => (
    title: 'Your ID could not be verified',
    body: 'Play is not available. Please contact support if you think this is a mistake.',
    action: null,
    route: null,
  ),
  PlayBlockReason.phoneNotProved => (
    title: 'Confirm your number with a first top-up',
    body: 'Approving the M-Pesa prompt of your first top-up proves the number is yours.',
    action: 'Top up',
    route: '/wallet/top-up',
  ),
  PlayBlockReason.inDebt => (
    title: 'Play is paused',
    body: 'A top-up was reversed by M-Pesa and money is owed. Your next top-up repays it first.',
    action: 'Open wallet',
    route: '/wallet',
  ),
  PlayBlockReason.dailyLimit => (
    title: "You've played $dailyCap times today",
    body: 'Plays reset at midnight Nairobi time.',
    action: 'See draws',
    route: '/draws',
  ),
  PlayBlockReason.lowBalance => (
    title: 'Your balance is below the price of a play',
    body: 'A play costs ${formatKes(priceKes)}. Top up with M-Pesa to play.',
    action: 'Top up',
    route: '/wallet/top-up',
  ),
};

/// The words for a confirmed play. Never "win": a play earns draw chances; prizes only come from draws.
({String heading, String detail, bool lucky}) describeResult(PlayResult r) {
  final spin = r.play.spin;
  final luckyBefore = r.today.tickets.any((t) => t.ticketId != r.play.ticketId && t.spin.lucky);
  final chances = r.today.chances == 1 ? '1 draw chance' : '${r.today.chances} draw chances';
  final heading = !spin.lucky
      ? 'Play recorded'
      : luckyBefore
      ? 'Lucky slot — today was already lucky'
      : "Lucky slot — today's chances are doubled";
  final detail =
      'Play ${r.play.seq} of ${r.today.plays + r.today.playsLeft} · slot ${spin.slot} of ${spin.slots}'
      '${spin.lucky ? ' (lucky)' : ''}. You now have $chances today.';
  return (heading: heading, detail: detail, lucky: spin.lucky);
}
