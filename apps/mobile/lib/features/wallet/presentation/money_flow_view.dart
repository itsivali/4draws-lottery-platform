import 'package:flutter/material.dart';

import '../../../app/theme/tokens.dart';
import '../../../core/widgets/announce.dart';
import '../../../core/widgets/failure_text.dart';
import '../../../core/widgets/status_pill.dart';
import '../application/settlement_flow.dart';

/// Shows where a money request stands, from the server's answers only.
class SettlementStatusView<T> extends StatefulWidget {
  const SettlementStatusView({
    super.key,
    required this.flow,
    required this.describe,
    required this.pendingText,
    required this.onDone,
  });

  final SettlementFlow<T> flow;

  /// The final state in words: tone, short label, sentence.
  final (StatusTone, String, String) Function(T item) describe;

  /// What we are waiting for (e.g. "Approve the M-Pesa prompt on your phone").
  final String pendingText;
  final VoidCallback onDone;

  @override
  State<SettlementStatusView<T>> createState() => _SettlementStatusViewState<T>();
}

class _SettlementStatusViewState<T> extends State<SettlementStatusView<T>> {
  SettlementStage? _announced;

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    return ValueListenableBuilder<SettlementState<T>>(
      valueListenable: widget.flow.state,
      builder: (context, s, _) {
        if (_announced != s.stage) {
          _announced = s.stage;
          final msg = switch (s.stage) {
            SettlementStage.waiting => widget.pendingText,
            SettlementStage.settled => widget.describe(s.item as T).$2,
            SettlementStage.refused => describeFailure(s.failure!).title,
            _ => null,
          };
          if (msg != null) WidgetsBinding.instance.addPostFrameCallback((_) => mounted ? announce(context, msg) : null);
        }
        final Widget body = switch (s.stage) {
          SettlementStage.ready => const SizedBox.shrink(),
          SettlementStage.sending => _Stage(
            tone: StatusTone.pending,
            busy: true,
            label: 'Sending',
            children: [Text('Sending your request…', style: t.bodyLarge)],
          ),
          SettlementStage.sendUncertain => _Stage(
            tone: StatusTone.pending,
            busy: s.checking,
            label: 'Checking your request',
            children: [
              Text(
                "The connection dropped before the server answered. Checking refers to the same request, so it can't "
                'be made twice. Please do not start another one.',
                style: t.bodyMedium,
              ),
              if (!s.checking) ...[
                const SizedBox(height: Space.md),
                Align(
                  alignment: Alignment.centerLeft,
                  child: OutlinedButton(onPressed: widget.flow.checkSend, child: const Text('Check again')),
                ),
              ],
            ],
          ),
          SettlementStage.refused => _Stage(
            tone: StatusTone.error,
            label: describeFailure(s.failure!).title,
            children: [
              if (describeFailure(s.failure!).detail case final d?) Text(d, style: t.bodyMedium),
              Text(
                s.retried
                    ? 'The first attempt may still have started it: check your wallet before trying again.'
                    : 'Nothing was started.',
                style: t.bodySmall,
              ),
            ],
          ),
          SettlementStage.waiting || SettlementStage.stillWaiting => _Stage(
            tone: StatusTone.pending,
            busy: s.checking || s.stage == SettlementStage.waiting,
            label: 'Pending',
            children: [
              Text(widget.pendingText, style: t.bodyLarge),
              const SizedBox(height: Space.xs),
              Text(
                s.stage == SettlementStage.stillWaiting
                    ? "It hasn't been confirmed yet. It's safe to leave: it shows in your wallet and updates when you come back."
                    : "We'll update this as soon as the server confirms it. It's safe to leave this screen.",
                style: t.bodySmall,
              ),
              const SizedBox(height: Space.sm),
              Align(
                alignment: Alignment.centerLeft,
                child: TextButton(
                  onPressed: s.checking ? null : widget.flow.refresh,
                  child: const Text('Check status now'),
                ),
              ),
            ],
          ),
          SettlementStage.settled => _Stage(
            tone: widget.describe(s.item as T).$1,
            label: widget.describe(s.item as T).$2,
            children: [
              Text(widget.describe(s.item as T).$3, style: t.bodyLarge),
              const SizedBox(height: Space.lg),
              FilledButton(onPressed: widget.onDone, child: const Text('Back to wallet')),
            ],
          ),
        };
        return Semantics(
          liveRegion: true,
          child: AnimatedSwitcher(
            duration: context.reduceMotion ? Duration.zero : Motion.standard,
            child: KeyedSubtree(key: ValueKey(s.stage), child: body),
          ),
        );
      },
    );
  }
}

/// One stage of a settlement: a toned icon disc, the state as a pill, and what it means.
class _Stage extends StatelessWidget {
  const _Stage({required this.tone, required this.label, required this.children, this.busy = false});

  final StatusTone tone;
  final String label;
  final List<Widget> children;
  final bool busy;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    final (color, icon) = tone.resolve(c);
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Container(
          width: 40,
          height: 40,
          decoration: BoxDecoration(color: color.withValues(alpha: 0.14), shape: BoxShape.circle),
          alignment: Alignment.center,
          child: busy
              ? SizedBox.square(dimension: 20, child: CircularProgressIndicator(strokeWidth: 2.5, color: color))
              : ExcludeSemantics(child: Icon(icon, color: color, size: 22)),
        ),
        const SizedBox(width: Space.md),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Align(
                alignment: Alignment.centerLeft,
                child: StatusPill(tone: tone, label: label),
              ),
              const SizedBox(height: Space.sm),
              ...children,
            ],
          ),
        ),
      ],
    );
  }
}
