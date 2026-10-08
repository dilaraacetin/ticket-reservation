import { router, useFocusEffect } from 'expo-router';
import { useCallback, useState } from 'react';
import { ScrollView, StyleSheet, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Banner, Button, Card, Caption, Empty, Heading, Label, Muted, Title } from '@/components/ui';
import { api } from '@/lib/api';
import { useSession } from '@/lib/session';
import { color, space } from '@/lib/theme';

export default function AccountScreen() {
  const { account, ready, signOut, refresh } = useSession();
  const [sent, setSent] = useState(false);
  const [sending, setSending] = useState(false);

  useFocusEffect(
    useCallback(() => {
      refresh();
    }, [refresh]),
  );

  if (!ready) return <SafeAreaView style={st.safe} edges={['top']} />;

  if (!account) {
    return (
      <SafeAreaView style={st.safe} edges={['top']}>
        <View style={st.pad}>
          <Title>Account</Title>
          <Empty
            icon="person-circle-outline"
            title="You are not signed in"
            text="Sign in to hold seats and keep your tickets in one place."
            action={{ label: 'Sign in', onPress: () => router.push('/sign-in') }}
          />
        </View>
      </SafeAreaView>
    );
  }

  const resend = async () => {
    setSending(true);
    try {
      await api('POST', '/auth/verify/resend');
      setSent(true);
    } finally {
      setSending(false);
    }
  };

  return (
    <SafeAreaView style={st.safe} edges={['top']}>
      <ScrollView contentContainerStyle={st.pad}>
        <Title>Account</Title>

        <Card style={st.who}>
          <View style={st.avatar}>
            <Text style={st.initial}>{account.email.slice(0, 1).toUpperCase()}</Text>
          </View>
          <View style={st.fill}>
            <Label>EMAIL</Label>
            <Heading>{account.email}</Heading>
          </View>
        </Card>

        {!account.emailVerified ? (
          <Card style={st.warn}>
            <Heading>Confirm your email</Heading>
            <Muted style={st.gap}>
              We sent a link to {account.email}. Holding a seat needs a confirmed address.
            </Muted>
            {sent ? (
              <Banner kind="success" title="On its way" text="Check your inbox, and the spam folder." />
            ) : null}
            <Button title="Send the link again" variant="secondary" onPress={resend} busy={sending} />
          </Card>
        ) : null}

        {account.role === 'admin' ? (
          <Card style={st.gap}>
            <Label>ROLE</Label>
            <Heading>Administrator</Heading>
            <Muted>Stocking and withdrawing events is done from the web interface.</Muted>
          </Card>
        ) : null}

        <Button title="Sign out" variant="danger" icon="log-out-outline" onPress={signOut} style={st.out} />
        <Caption style={st.note}>Signs out on this device only. Your other devices stay signed in.</Caption>
      </ScrollView>
    </SafeAreaView>
  );
}

const st = StyleSheet.create({
  safe: { flex: 1, backgroundColor: color.bg },
  pad: { padding: space.gutter, paddingBottom: 32 },
  fill: { flex: 1 },

  who: { flexDirection: 'row', alignItems: 'center', gap: 16, marginTop: 18 },
  avatar: {
    width: 64,
    height: 64,
    borderRadius: 32,
    backgroundColor: color.surfaceRaised,
    alignItems: 'center',
    justifyContent: 'center',
  },
  initial: { fontSize: 26, fontWeight: '700', color: color.text },

  warn: { marginTop: 14, borderColor: color.warning, backgroundColor: color.warningBg, gap: 10 },
  gap: { marginTop: 14 },

  out: { marginTop: 28 },
  note: { textAlign: 'center', marginTop: 10 },
});
