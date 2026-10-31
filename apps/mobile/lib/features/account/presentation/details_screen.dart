import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../app/theme/tokens.dart';
import '../../../core/api/api_failure.dart';
import '../../../core/api/idempotency.dart';
import '../../../core/widgets/app_card.dart';
import '../../../core/widgets/async_view.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/content_frame.dart';
import '../../../core/widgets/failure_text.dart';
import '../../../core/widgets/inline_banner.dart';
import '../../../core/widgets/status_pill.dart';
import '../../auth/presentation/identity_fields.dart';
import '../application/account_providers.dart';
import '../domain/account_models.dart';

/// The player's names and ID number (owner, 2026-10-31). An account that gave none at sign-up fills them in here,
/// once, with the sign-up fields; afterwards they are shown, the number masked, with where they stand.
class DetailsScreen extends ConsumerStatefulWidget {
  const DetailsScreen({super.key});

  @override
  ConsumerState<DetailsScreen> createState() => _DetailsScreenState();
}

class _DetailsScreenState extends ConsumerState<DetailsScreen> {
  final _form = GlobalKey<FormState>();
  final _firstName = TextEditingController();
  final _secondName = TextEditingController();
  final _nationalId = TextEditingController();
  var _busy = false;
  String? _idError;
  (StatusTone, String, String?)? _notice;
  String? _key;
  String? _sentBody; // the key is reused only for the same details; edited details are a new request

  @override
  void dispose() {
    _firstName.dispose();
    _secondName.dispose();
    _nationalId.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    setState(() {
      _idError = null;
      _notice = null;
    });
    if (!_form.currentState!.validate()) return;
    setState(() => _busy = true);
    final body = '${_firstName.text.trim()}|${_secondName.text.trim()}|${_nationalId.text.trim()}';
    if (_sentBody != body) _key = null;
    _sentBody = body;
    _key ??= newIdempotencyKey(); // the same key on a retry after a lost answer
    try {
      await ref
          .read(accountRepositoryProvider)
          .declareIdentity(
            firstName: _firstName.text.trim(),
            secondName: _secondName.text.trim(),
            nationalId: _nationalId.text.trim(),
            idempotencyKey: _key!,
          );
      _key = null;
      ref
        ..invalidate(accountProvider)
        ..invalidate(declaredIdentityProvider);
      await ref.read(accountProvider.future);
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Your details are saved.')));
      context.go('/account');
    } on ApiFailure catch (f) {
      if (!f.outcomeUncertain) _key = null;
      if (!mounted) return;
      setState(() {
        switch (f) {
          case ProblemFailure(slug: 'invalid-identity', :final title, :final detail):
            _idError = [title, ?detail].join('. ');
          case ProblemFailure(slug: 'identity-declared' || 'kyc-state', :final title, :final detail):
            // the account already has them (another device, or a submission): show what it has
            _notice = (StatusTone.neutral, title, detail);
            ref.invalidate(accountProvider);
          default:
            final d = describeFailure(f);
            _notice = (
              StatusTone.error,
              f.outcomeUncertain ? '${d.title}. Save again to check: it will not be sent twice.' : d.title,
              f.outcomeUncertain ? null : d.detail,
            );
        }
      });
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final account = ref.watch(accountProvider);
    return PageScaffold(
      title: 'Your details',
      onRefresh: () async => ref.invalidate(accountProvider),
      children: [
        if (_notice case (final tone, final title, final message)) ...[
          InlineBanner(tone: tone, live: true, title: title, message: message),
          const SizedBox(height: Space.lg),
        ],
        AsyncView(
          value: account,
          what: 'your details',
          onRetry: () => ref.invalidate(accountProvider),
          data: (a) => a.needsIdentity ? _formCard(context) : _factsCard(context, a),
        ),
      ],
    );
  }

  Widget _formCard(BuildContext context) {
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    return Form(
      key: _form,
      child: AppCard(
        child: AutofillGroup(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  ExcludeSemantics(child: Icon(Icons.badge_outlined, color: c.primary)),
                  const SizedBox(width: Space.md),
                  Expanded(
                    child: Text(
                      'Your account has no names yet. Give your first name, second name and national ID number exactly '
                      'as on your ID, the way M-Pesa shows them: staff check them before your first withdrawal, and '
                      'they cannot be changed here afterwards. They are stored encrypted.',
                      style: t.bodyMedium?.copyWith(color: c.inkMuted),
                    ),
                  ),
                ],
              ),
              const SizedBox(height: Space.xl),
              IdentityFields(firstName: _firstName, secondName: _secondName, nationalId: _nationalId, error: _idError),
              const SizedBox(height: Space.xl),
              BusyButton(label: 'Save my details', busy: _busy, onPressed: _submit),
            ],
          ),
        ),
      ),
    );
  }

  Widget _factsCard(BuildContext context, Account a) {
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    final (tone, standing) = switch (a.identity?.status) {
      IdentityStatus.verified => (StatusTone.success, 'Verified'),
      IdentityStatus.submitted => (StatusTone.pending, 'Being checked by staff'),
      _ => (StatusTone.neutral, 'Checked before your first withdrawal'),
    };
    return AppCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          FactGrid(
            facts: [
              FactRow(label: 'First name', value: a.firstName ?? '—'),
              FactRow(label: 'Second name', value: a.secondName ?? '—'),
              FactRow(label: 'National ID number', value: a.identity?.maskedNationalId ?? '—', mono: true),
            ],
          ),
          const SizedBox(height: Space.lg),
          Align(
            alignment: Alignment.centerLeft,
            child: StatusPill(tone: tone, label: standing),
          ),
          const SizedBox(height: Space.lg),
          Text(
            'These must match your national ID and the name M-Pesa shows for your number. If something is wrong, '
            'contact support before you verify your identity.',
            style: t.bodySmall?.copyWith(color: c.inkMuted),
          ),
        ],
      ),
    );
  }
}
