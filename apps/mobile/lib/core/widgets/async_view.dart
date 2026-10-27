import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

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
    if (v.hasValue) return data(v.requireValue);
    if (v.hasError) return ErrorNotice(error: v.error!, onRetry: onRetry, what: what);
    return loading ?? const SkeletonLines();
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
