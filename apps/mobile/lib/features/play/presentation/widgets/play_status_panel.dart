import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../../../../app/theme/tokens.dart';
import '../../../../core/api/api_failure.dart';
import '../../../../core/formatting/money.dart';
import '../../../../core/widgets/depth.dart';
import '../../../../core/widgets/failure_text.dart';
import '../../application/play_controller.dart';
import '../play_copy.dart';
import 'play_receipt_sheet.dart';

/// Tells the player, in words, where the play stands. This (not the wheel) is what screen readers read.
class PlayStatusPanel extends StatelessWidget {
  const PlayStatusPanel({
    super.key,
    required this.flow,
    required this.onDone,
    required this.onCheckAgain,
    required this.onForget,
    required this.onSetAside,
  });

  final PlayFlow flow;
  final VoidCallback onDone;
  final VoidCallback onCheckAgain;
  final VoidCallback onForget;
  final VoidCallback onSetAside;

  @override
  Widget build(BuildContext context) {
    final child = switch (flow.stage) {
      PlayStage.idle => const SizedBox.shrink(key: ValueKey('idle')),
      PlayStage.submitting || PlayStage.landing => _Line(
        key: const ValueKey('spinning'),
        icon: Icons.donut_large_rounded,
        tone: context.colors.pending,
        title: flow.stage == PlayStage.submitting
            ? 'Recording your play…'
            : 'Play recorded — the wheel is showing your spin',
        busy: true,
      ),
      PlayStage.revealed => _Result(key: ValueKey('result-${flow.result!.play.ticketId}'), flow: flow, onDone: onDone),
      PlayStage.refused => _Refused(
        key: const ValueKey('refused'),
        failure: flow.failure!,
        recovered: flow.recovered,
        onDone: onDone,
      ),
      PlayStage.uncertain => _Uncertain(
        key: const ValueKey('uncertain'),
        flow: flow,
        onCheckAgain: onCheckAgain,
        onForget: onForget,
        onSetAside: onSetAside,
      ),
    };
    return AnimatedSwitcher(
      duration: context.reduceMotion ? Duration.zero : Motion.emphasis,
      switchInCurve: Motion.enter,
      switchOutCurve: Motion.exit,
      transitionBuilder: (child, animation) => FadeTransition(
        opacity: animation,
        child: SlideTransition(
          position: Tween(begin: const Offset(0, 0.04), end: Offset.zero).animate(animation),
          child: child,
        ),
      ),
      child: child,
    );
  }
}

class _Line extends StatelessWidget {
  const _Line({super.key, required this.icon, required this.tone, required this.title, this.busy = false});

  final IconData icon;
  final Color tone;
  final String title;
  final bool busy;

  @override
  Widget build(BuildContext context) => Semantics(
    liveRegion: true,
    child: Container(
      padding: const EdgeInsets.symmetric(horizontal: Space.lg, vertical: Space.md),
      decoration: BoxDecoration(color: tone.withValues(alpha: 0.07), borderRadius: BorderRadius.circular(Radii.lg)),
      child: Row(
        children: [
          if (busy)
            SizedBox.square(dimension: 20, child: CircularProgressIndicator(strokeWidth: 2.5, color: tone))
          else
            ExcludeSemantics(child: Icon(icon, color: tone)),
          const SizedBox(width: Space.md),
          Expanded(child: Text(title, style: Theme.of(context).textTheme.bodyLarge)),
        ],
      ),
    ),
  );
}

class _Panel extends StatelessWidget {
  const _Panel({required this.icon, required this.tone, required this.title, required this.children});

  final IconData icon;
  final Color tone;
  final String title;
  final List<Widget> children;

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    return Container(
      padding: const EdgeInsets.all(Space.xl),
      decoration: BoxDecoration(
        color: c.surface,
        borderRadius: BorderRadius.circular(Radii.card),
        border: Border.all(color: tone.withValues(alpha: 0.45)),
        boxShadow: Elevation.card(c),
      ),
      child: Semantics(
        liveRegion: true,
        container: true,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                // a floating 3D orb in the panel's tone (not a coin: a play wins chances, never money)
                Floating(
                  amplitude: 2.5,
                  child: Orb(icon: icon, size: 40, color: tone),
                ),
                const SizedBox(width: Space.md),
                Expanded(
                  child: Padding(
                    padding: const EdgeInsets.only(top: Space.xs),
                    child: Semantics(header: true, child: Text(title, style: t.titleMedium)),
                  ),
                ),
              ],
            ),
            const SizedBox(height: Space.md),
            ...children,
          ],
        ),
      ),
    );
  }
}

class _Result extends StatelessWidget {
  const _Result({super.key, required this.flow, required this.onDone});

  final PlayFlow flow;
  final VoidCallback onDone;

  @override
  Widget build(BuildContext context) {
    final r = flow.result!;
    final d = describeResult(r);
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    return _Panel(
      icon: d.lucky ? Icons.star_rounded : Icons.check_circle_rounded,
      tone: d.lucky ? c.goldText : c.success,
      title: d.heading,
      children: [
        Text(d.detail, style: t.bodyMedium),
        const SizedBox(height: Space.xs),
        Text(
          'Each chance is one entry in every draw whose period includes today. This is not a prize: prizes are '
          'decided only in the draws.',
          style: t.bodySmall,
        ),
        if (flow.recovered) ...[
          const SizedBox(height: Space.sm),
          Text(
            'We confirmed this play after the connection dropped. It was charged once.',
            style: t.bodySmall?.copyWith(color: c.pending),
          ),
        ],
        const SizedBox(height: Space.sm),
        Text('Balance after this play: ${formatKes(r.balanceKes)}', style: t.bodySmall),
        const SizedBox(height: Space.md),
        Wrap(
          spacing: Space.sm,
          runSpacing: Space.sm,
          children: [
            OutlinedButton(onPressed: () => showPlayReceipt(context, r.play), child: const Text('View receipt')),
            TextButton(onPressed: onDone, child: const Text('Done')),
          ],
        ),
      ],
    );
  }
}

class _Refused extends StatelessWidget {
  const _Refused({super.key, required this.failure, required this.recovered, required this.onDone});

  final ApiFailure failure;

  /// The refusal answered a replay after a lost answer, so the first attempt is not claimed either way.
  final bool recovered;
  final VoidCallback onDone;

  @override
  Widget build(BuildContext context) {
    final d = describeFailure(failure);
    final t = Theme.of(context).textTheme;
    final route = switch (failure) {
      ProblemFailure(slug: 'insufficient-funds' || 'phone-not-verified') => ('Top up', '/wallet/top-up'),
      ProblemFailure(slug: 'kyc-required') => ('Verify my ID', '/account/verify'),
      _ => null,
    };
    return _Panel(
      icon: Icons.error_rounded,
      tone: context.colors.error,
      title: d.title,
      children: [
        if (d.detail != null) Text(d.detail!, style: t.bodyMedium),
        Text(
          recovered
              ? "This answers a check of an earlier play. If that play did go through, it shows in today's plays."
              : 'Nothing was charged.',
          style: t.bodySmall,
        ),
        const SizedBox(height: Space.md),
        Wrap(
          spacing: Space.sm,
          children: [
            if (route != null) OutlinedButton(onPressed: () => context.go(route.$2), child: Text(route.$1)),
            TextButton(onPressed: onDone, child: const Text('OK')),
          ],
        ),
      ],
    );
  }
}

class _Uncertain extends StatelessWidget {
  const _Uncertain({
    super.key,
    required this.flow,
    required this.onCheckAgain,
    required this.onForget,
    required this.onSetAside,
  });

  final PlayFlow flow;
  final VoidCallback onCheckAgain;
  final VoidCallback onForget;
  final VoidCallback onSetAside;

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    if (flow.needsConsent) {
      return _Panel(
        icon: Icons.schedule_rounded,
        tone: c.pending,
        title: "An earlier play didn't finish",
        children: [
          Text(
            'The app lost the answer to a play you started earlier. Checking asks the server about that same play: '
            'if it went through you will see it; if it never arrived, checking completes it — one play, charged once.',
            style: t.bodyMedium,
          ),
          const SizedBox(height: Space.md),
          Wrap(
            spacing: Space.sm,
            runSpacing: Space.sm,
            children: [
              FilledButton(onPressed: flow.checking ? null : onCheckAgain, child: const Text('Check that play')),
              TextButton(onPressed: flow.checking ? null : onForget, child: const Text('Forget it')),
            ],
          ),
          Text("If you forget it, any play that did go through still shows in today's plays.", style: t.bodySmall),
        ],
      );
    }
    return _Panel(
      icon: Icons.schedule_rounded,
      tone: c.pending,
      title: "We're checking whether your play went through",
      children: [
        Text(
          "The connection dropped before the server's answer arrived. Every check refers to the same play, so you can "
          'never be charged twice.',
          style: t.bodyMedium,
        ),
        const SizedBox(height: Space.md),
        if (flow.checking || !flow.autoChecksExhausted)
          Row(
            children: [
              SizedBox.square(dimension: 18, child: CircularProgressIndicator(strokeWidth: 2.5, color: c.pending)),
              const SizedBox(width: Space.md),
              Expanded(child: Text(flow.checking ? 'Checking…' : 'Checking again shortly…', style: t.bodySmall)),
            ],
          )
        else ...[
          Text("We couldn't confirm it yet. It is safe to leave and come back.", style: t.bodySmall),
          const SizedBox(height: Space.sm),
          Wrap(
            spacing: Space.sm,
            runSpacing: Space.sm,
            children: [
              OutlinedButton(onPressed: onCheckAgain, child: const Text('Check again')),
              TextButton(onPressed: onSetAside, child: const Text('Set aside for now')),
            ],
          ),
        ],
      ],
    );
  }
}
