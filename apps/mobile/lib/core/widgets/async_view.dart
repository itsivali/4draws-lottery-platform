import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../app/theme/tokens.dart';
import '../api/api_failure.dart';
import 'failure_text.dart';
import 'skeleton.dart';

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
    final c = context.colors;
    final t = Theme.of(context).textTheme;
    final d = error is ApiFailure ? describeFailure(error as ApiFailure) : (title: "Couldn't load $what", detail: null);
    return Semantics(
      liveRegion: true,
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          ExcludeSemantics(child: Icon(Icons.error_rounded, color: c.error)),
          const SizedBox(width: Space.md),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(error is ApiFailure ? "Couldn't load $what. ${d.title}" : d.title, style: t.bodyMedium),
                if (d.detail != null) Text(d.detail!, style: t.bodySmall),
                if (onRetry != null)
                  Padding(
                    padding: const EdgeInsets.only(top: Space.sm),
                    child: OutlinedButton.icon(
                      onPressed: onRetry,
                      icon: const Icon(Icons.refresh_rounded),
                      label: const Text('Try again'),
                    ),
                  ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
