import { Ionicons } from '@expo/vector-icons';
import { Tabs } from 'expo-router';

import { color } from '@/lib/theme';

export default function TabsLayout() {
  return (
    <Tabs
      screenOptions={{
        headerShown: false,
        tabBarActiveTintColor: color.primary,
        tabBarInactiveTintColor: color.textMuted,
        // The bar measures its own height here, which is the thing the web
        // stylesheet had to be taught by hand four times over.
        tabBarStyle: { backgroundColor: color.surface, borderTopColor: color.line },
        tabBarLabelStyle: { fontSize: 12, fontWeight: '600' },
        sceneStyle: { backgroundColor: color.bg },
      }}>
      <Tabs.Screen
        name="index"
        options={{
          title: 'Events',
          tabBarIcon: ({ color: tint, size }) => <Ionicons name="calendar" size={size} color={tint} />,
        }}
      />
      <Tabs.Screen
        name="tickets"
        options={{
          title: 'My tickets',
          tabBarIcon: ({ color: tint, size }) => <Ionicons name="ticket" size={size} color={tint} />,
        }}
      />
      <Tabs.Screen
        name="account"
        options={{
          title: 'Account',
          tabBarIcon: ({ color: tint, size }) => <Ionicons name="person" size={size} color={tint} />,
        }}
      />
    </Tabs>
  );
}
