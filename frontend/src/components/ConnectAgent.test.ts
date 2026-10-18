import { claudeAddCommand } from './ConnectAgent';

describe('claudeAddCommand', () => {
  it('registers CodeDiver for every project by default', () => {
    expect(claudeAddCommand('http://127.0.0.1:47474/mcp')).toBe('claude mcp add --transport http --scope user codediver http://127.0.0.1:47474/mcp');
  });

  it('can register it for the team through .mcp.json', () => {
    expect(claudeAddCommand('http://127.0.0.1:5000/mcp', 'project')).toBe('claude mcp add --transport http --scope project codediver http://127.0.0.1:5000/mcp');
  });
});
