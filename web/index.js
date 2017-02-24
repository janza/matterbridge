import { h, render, Component } from 'preact'
import {
  Button,
  Comment,
  Dropdown,
  Form,
  Grid,
  Header,
  Icon,
  Input,
  Menu,
  Segment,
  Sidebar,
} from 'semantic-ui-react'

import {HotKeys} from 'react-hotkeys'

var ws = new WebSocket('ws://localhost:8001/ws')

var state = {
  messages: [],
  activeChannel: {},
  channels: {},
  users: {},
}

const newChannel = (channels, account, channel) => {
  return channels.filter((c) => c.account === account && c.channel === channel)[0] || {
    account, channel
  };
}

ws.addEventListener('message', function (msg, flags) {
  const data = JSON.parse(msg.data)

  if (data.Type === 'message') {
    updateState(Object.assign({}, state, {
      messages: state.messages.concat([
        data.Message
      ]).sort(
        (a, b) => a.Timestamp < b.Timestamp ? -1 : (a.Timestamp == b.Timestamp ? 0 : 1)
      ),
    }))
  } else if (data.Type === 'user')  {
    updateState(
      Object.assign(
        {},
        state,
        {
          users: Object.assign(
            {},
            state.users, {
              [data.User.ID]: data.User
            }
          )
        }
      )
    )
  } else if (data.Type === 'channel')  {
    updateState(
      Object.assign(
        {},
        state,
        {
          channels: Object.assign(
          {},
          state.channels, {
            [data.Channel.ID]: data.Channel
          }
          )
        }
      )
    )
  }
})

ws.addEventListener('close', function () {
  ws = new WebSocket('ws://localhost:8001/ws')
});

var stateThrottle;

function updateState (newState) {
  window.state = state = newState
  clearTimeout(stateThrottle);
  stateThrottle = setTimeout(() => redraw(state), 100);
  // redraw(state)
}

function setActiveChannel (channel) {
  if (channel.ID === state.activeChannel.ID) return
  updateState(Object.assign({}, state, {
    activeChannel: channel
  }))
  sendCommand("replay_messages", channel.ID)
}

function sendCommand (Command, Param) {
  ws.send(JSON.stringify({
    Command: { Command, Param }
  }))
}

function sendMessage ({Account, Channel, User}, text) {
  ws.send(JSON.stringify({
    Message: {
      Channel: Channel || User,
      To: Account,
      Text: text
    }
  }))
}

const Feed = ({messages, activeChannel, users}) => {
  var previousUsername;
  return <Comment.Group minimal>
    { messages.filter(
      m => {
        return (
          m.Channel === activeChannel.Channel
          || m.Channel === activeChannel.User
        ) && (
          m.Account === activeChannel.Account
          || m.To === activeChannel.Account
        )
      }
    ).map((m) => {
      const differentUser = m.Username !== previousUsername
      previousUsername = m.Username
      const user = users[`${m.Username}:${m.Account}`]
      const userName = user && user.Name || m.Username
      return <Comment>
        {
          differentUser
            ? <Comment.Avatar as='a' src={'https://robohash.org/' + m.Username} />
            : <Comment.Avatar />
        }
        <Comment.Content>
          { differentUser ? <Comment.Author as='a'>{ userName }</Comment.Author> : null }
          { differentUser
              ?<Comment.Metadata>
                <span>{ m.Timestamp }</span>
              </Comment.Metadata>
              : null }
          <Comment.Text>{ m.Text }</Comment.Text>
        </Comment.Content>
      </Comment>
    }) }
  </Comment.Group>
}

class Channels extends Component {

  constructor() {
    super()
    this.state.filter = ''
  }

  focusChannelSearch() {
    this.channelSearch.focus()
  }

  searchChannel(e) {
    const filter = e.target.value
    this.setState({filter: filter})
    const newActiveChannel = this.props.channels.filter(c => {
      return !this.state.filter || c.ID.indexOf(this.state.filter) >= 0
    })[0];

    if (!newActiveChannel) return
    setActiveChannel(newActiveChannel)
  }

  render({channels, activeChannel, visible}) {
    return <Sidebar as={Menu} animation='push' visible={visible} icon='labeled' vertical inverted>
      <Menu.Item>
        <Input placeholder='Search...' >
          <input
            type='text'
            ref={(el) => { this.channelSearch = el }}
            onKeyup={(e) => this.searchChannel(e)}
          />
        </Input>
      </Menu.Item>
      {
        channels.filter(c => {
          return c.ID && (!this.state.filter || c.ID.indexOf(this.state.filter) >= 0)
        }).map(c => (
          <Menu.Item onClick={() => setActiveChannel(c)} active={c == activeChannel} >
            <span style={{
              maxWidth: '140px',
              overflow: 'hidden',
              display: 'inline-block'
            }}>
            {c.Name || c.ID}
            </span>
          </Menu.Item>
        ))
      }
    </Sidebar>
  }
}

const MessageInput = ({activeChannel}) => {
  return <Form onSubmit={(e, data) => {
    e.preventDefault()
    sendMessage(activeChannel, data.formData.msg)
    e.target.firstChild.firstChild.value = ''
  }}>
    <Input fluid name='msg' action='Send' placeholder='Type in something...' />
  </Form>

}

const keyMap = {
  'focusChannelSearch': 'alt+k'
}

class App extends Component {
  render({state}) {

    return <HotKeys
      keyMap={keyMap}
      style={{height: '100%'}}
      handlers={{
        'focusChannelSearch': () => this.channels.focusChannelSearch()
      }}>
      <Sidebar.Pushable as={Segment}>
        <Channels
          ref={(el) => this.channels = el}
          visible
          channels={Object.values(state.channels).concat(Object.values(state.users))}
          activeChannel={state.activeChannel} />
        <Sidebar.Pusher style={{ maxHeight: '100%', overflowY: 'scroll'}}>
          <Header as='h2' content={state.activeChannel.Name} />
          <Feed messages={state.messages} users={state.users} activeChannel={state.activeChannel} />
          <MessageInput activeChannel={state.activeChannel} />
        </Sidebar.Pusher>
      </Sidebar.Pushable>
    </HotKeys>
  }
}

function redraw ({messages}) {
  const chat = document.querySelector('#chat')
  chat.style.height = '100%';
  render((<App state={state}/>), chat, chat.lastChild)
}

redraw(state)
