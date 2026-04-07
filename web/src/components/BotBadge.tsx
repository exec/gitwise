interface BotBadgeProps {
  isBot?: boolean;
}

export default function BotBadge({ isBot }: BotBadgeProps) {
  if (!isBot) return null;
  return <span className="bot-badge">bot</span>;
}
