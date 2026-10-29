import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../app/theme/tokens.dart';
import '../../../core/api/api_failure.dart';
import '../../../core/api/idempotency.dart';
import '../../../core/widgets/app_card.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/content_frame.dart';
import '../../../core/widgets/failure_text.dart';
import '../../../core/widgets/inline_banner.dart';
import '../../../core/widgets/status_pill.dart';
import '../application/account_providers.dart';

/// Change, or add, the first and second names the app calls the player by (not the legal name, which the identity
/// submission carries).
class NameScreen extends ConsumerStatefulWidget {
  const NameScreen({super.key});

  @override
  ConsumerState<NameScreen> createState() => _NameScreenState();
}

class _NameScreenState extends ConsumerState<NameScreen> {
  final _form = GlobalKey<FormState>();
  final _first = TextEditingController();
  final _second = TextEditingController();
  var _busy = false;
  var _prefilled = false;
  String? _error;

  @override
  void dispose() {
    _first.dispose();
    _second.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    setState(() => _error = null);
    if (!_form.currentState!.validate()) return;
    setState(() => _busy = true);
    try {
      await ref
          .read(accountRepositoryProvider)
          .setName(first: _first.text.trim(), second: _second.text.trim(), idempotencyKey: newIdempotencyKey());
      ref.invalidate(accountProvider);
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Name saved.')));
      context.go('/account');
    } on ApiFailure catch (f) {
      if (!mounted) return;
      final d = describeFailure(f);
      setState(() => _error = [d.title, ?d.detail].join(': '));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final account = ref.watch(accountProvider).value;
    if (!_prefilled && account != null) {
      _prefilled = true;
      _first.text = account.firstName ?? '';
      _second.text = account.secondName ?? '';
    }
    final adding = account != null && account.displayName == null;
    return PageScaffold(
      title: adding ? 'Add your name' : 'Your name',
      children: [
        Form(
          key: _form,
          child: AppCard(
            child: AutofillGroup(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Text(
                    'How the app addresses you. Your legal name is the one on your national ID, which identity verification checks.',
                    style: Theme.of(context).textTheme.bodyMedium,
                  ),
                  const SizedBox(height: Space.lg),
                  TextFormField(
                    controller: _first,
                    textCapitalization: TextCapitalization.words,
                    autofillHints: const [AutofillHints.givenName],
                    textInputAction: TextInputAction.next,
                    decoration: const InputDecoration(labelText: 'First name', prefixIcon: Icon(Icons.person_rounded)),
                    validator: (v) => (v ?? '').trim().isEmpty ? 'Enter your first name.' : null,
                  ),
                  const SizedBox(height: Space.lg),
                  TextFormField(
                    controller: _second,
                    textCapitalization: TextCapitalization.words,
                    autofillHints: const [AutofillHints.familyName],
                    textInputAction: TextInputAction.done,
                    onFieldSubmitted: (_) => _submit(),
                    decoration: const InputDecoration(
                      labelText: 'Second name',
                      prefixIcon: Icon(Icons.person_outline_rounded),
                    ),
                    validator: (v) => (v ?? '').trim().isEmpty ? 'Enter your second name.' : null,
                  ),
                  if (_error != null) ...[
                    const SizedBox(height: Space.lg),
                    InlineBanner(tone: StatusTone.error, live: true, title: _error!),
                  ],
                  const SizedBox(height: Space.xl),
                  BusyButton(
                    label: adding ? 'Add name' : 'Save',
                    busy: _busy,
                    onPressed: account == null ? null : _submit,
                  ),
                ],
              ),
            ),
          ),
        ),
      ],
    );
  }
}
