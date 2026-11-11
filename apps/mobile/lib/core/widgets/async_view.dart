import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../app/theme/tokens.dart';
import '../api/api_failure.dart';
import 'failure_text.dart';
import 'inline_banner.dart';
import 'skeleton.dart';
import 'status_pill.dart';

/// Loading, error and data for one server read (MASTER.md §11). While refreshing it keeps showing the last
/// confirmed data instead of flashing a skeleton.
class AsyncView<T> extends StatelessWidget {
  const AsyncView({super.key, required this.value, required this.data, this.loading, this.onRetry, this.what = 'this'});

  final AsyncValue<T> value;
  final Widget Function(T) data;
  final Widget? loading;
  final VoidCallback? onRetry;

  /// Used in the error message ("Couldn't load your wallet").
  final String what;

  @override
  Widget build(BuildContext context) {
    final v = value;
    final (state, child) = v.hasValue
        ? (0, data(v.requireValue))
        : v.hasError
        ? (1, ErrorNotice(error: v.error!, onRetry: onRetry, what: what))
        : (2, loading ?? const SkeletonLines());
    if (context.reduceMotion) return child;
    // the skeleton gives way to the data in a short cross-fade, and the card grows to fit it, instead of a cut
    return AnimatedSize(
      duration: Motion.standard,
      curve: Motion.settle,
      alignment: Alignment.topCenter,
      child: AnimatedSwitcher(
        duration: Motion.standard,
        switchInCurve: Motion.enter,
        switchOutCurve: Motion.exit,
        layoutBuilder: (current, previous) => Stack(alignment: Alignment.topCenter, children: [...previous, ?current]),
        child: KeyedSubtree(key: ValueKey(state), child: child),
      ),
    );
  }
}

class ErrorNotice extends StatelessWidget {
  const ErrorNotice({super.key, required this.error, this.onRetry, this.what = 'this'});

  final Object error;
  final VoidCallback? onRetry;
  final String what;

  @override
  Widget build(BuildContext context) {
    final d = error is ApiFailure ? describeFailure(error as ApiFailure) : (title: "Couldn't load $what", detail: null);
    return InlineBanner(
      tone: StatusTone.error,
      live: true,
      title: error is ApiFailure ? "Couldn't load $what. ${d.title}" : d.title,
      message: d.detail,
      actionLabel: onRetry == null ? null : 'Try again',
      onAction: onRetry,
    );
  }
}
