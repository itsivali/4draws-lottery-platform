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
    final c = context.colors;
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
          SettlementStage.sending => _row(context, c.pending, 'Sending your request…', busy: true),
          SettlementStage.sendUncertain => Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const StatusPill(tone: StatusTone.pending, label: 'Checking your request'),
              const SizedBox(height: Space.sm),
              Text(
                "The connection dropped before the server answered. Checking refers to the same request, so it can't "
                'be made twice. Please do not start another one.',
                style: t.bodyMedium,
              ),
              const SizedBox(height: Space.sm),
              if (s.checking)
                _row(context, c.pending, 'Checking…', busy: true)
              else
                OutlinedButton(onPressed: widget.flow.checkSend, child: const Text('Check again')),
            ],
          ),
          SettlementStage.refused => Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              StatusPill(tone: StatusTone.error, label: describeFailure(s.failure!).title),
              if (describeFailure(s.failure!).detail case final d?)
                Padding(
                  padding: const EdgeInsets.only(top: Space.sm),
                  child: Text(d, style: t.bodyMedium),
                ),
              Text('Nothing was started.', style: t.bodySmall),
            ],
          ),
          SettlementStage.waiting || SettlementStage.stillWaiting => Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const StatusPill(tone: StatusTone.pending, label: 'Pending'),
              const SizedBox(height: Space.sm),
              Text(widget.pendingText, style: t.bodyLarge),
              const SizedBox(height: Space.xs),
              Text(
                s.stage == SettlementStage.stillWaiting
                    ? "It hasn't been confirmed yet. It's safe to leave: it shows in your wallet and updates when you come back."
                    : "We'll update this as soon as the server confirms it. It's safe to leave this screen.",
                style: t.bodySmall,
              ),
              const SizedBox(height: Space.sm),
              Row(
                children: [
                  if (s.checking || s.stage == SettlementStage.waiting) ...[
                    SizedBox.square(
                      dimension: 18,
                      child: CircularProgressIndicator(strokeWidth: 2.5, color: c.pending),
                    ),
                    const SizedBox(width: Space.md),
                  ],
                  TextButton(onPressed: s.checking ? null : widget.flow.refresh, child: const Text('Check status now')),
                ],
              ),
            ],
          ),
          SettlementStage.settled => Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              StatusPill(tone: widget.describe(s.item as T).$1, label: widget.describe(s.item as T).$2),
              const SizedBox(height: Space.sm),
              Text(widget.describe(s.item as T).$3, style: t.bodyLarge),
              const SizedBox(height: Space.md),
              FilledButton(onPressed: widget.onDone, child: const Text('Back to wallet')),
            ],
          ),
        };
        return Semantics(liveRegion: true, child: body);
      },
    );
  }

  Widget _row(BuildContext context, Color color, String text, {bool busy = false}) => Row(
    children: [
      if (busy) SizedBox.square(dimension: 18, child: CircularProgressIndicator(strokeWidth: 2.5, color: color)),
      const SizedBox(width: Space.md),
      Expanded(child: Text(text, style: Theme.of(context).textTheme.bodyLarge)),
    ],
  );
}
