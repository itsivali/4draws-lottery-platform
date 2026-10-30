import 'dart:async';

import 'package:flutter/material.dart';

import '../../../app/theme/tokens.dart';
import '../application/top_up_cooldown.dart';

/// Counts down to [readyAt] and says when the next M-Pesa prompt may be asked for; [builder] gets whether the wait is
/// over, so a button can stay off until then. Nothing is drawn by the note itself once the wait is over.
class TopUpCooldownGate extends StatefulWidget {
  const TopUpCooldownGate({super.key, required this.readyAt, required this.builder, this.what = 'top up'});

  final DateTime? readyAt;
  final Widget Function(BuildContext context, bool ready, Widget? note) builder;

  /// What the player may do again: "top up", "pay with M-Pesa".
  final String what;

  @override
  State<TopUpCooldownGate> createState() => _TopUpCooldownGateState();
}

class _TopUpCooldownGateState extends State<TopUpCooldownGate> {
  Timer? _tick;

  Duration get _left {
    final at = widget.readyAt;
    if (at == null) return Duration.zero;
    final d = at.difference(DateTime.now());
    return d.isNegative ? Duration.zero : d;
  }

  @override
  void initState() {
    super.initState();
    _schedule();
  }

  @override
  void didUpdateWidget(TopUpCooldownGate old) {
    super.didUpdateWidget(old);
    if (old.readyAt != widget.readyAt) _schedule();
  }

  void _schedule() {
    _tick?.cancel();
    if (_left > Duration.zero) {
      _tick = Timer.periodic(const Duration(seconds: 1), (_) {
        if (!mounted) return;
        setState(() {});
        if (_left == Duration.zero) _tick?.cancel();
      });
    }
  }

  @override
  void dispose() {
    _tick?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final left = _left;
    if (left == Duration.zero) return widget.builder(context, true, null);
    final c = context.colors;
    final note = Semantics(
      liveRegion: true,
      child: Row(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          ExcludeSemantics(child: Icon(Icons.hourglass_top_rounded, size: 18, color: c.pending)),
          const SizedBox(width: Space.xs),
          Flexible(
            child: Text(
              'You can ${widget.what} again in ${describeWait(left)}. M-Pesa answers one prompt at a time.',
              style: Theme.of(context).textTheme.bodySmall,
              textAlign: TextAlign.center,
            ),
          ),
        ],
      ),
    );
    return widget.builder(context, false, note);
  }
}
