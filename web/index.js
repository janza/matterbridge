import { h, render, Component } from 'preact'
import {
  Dropdown,
  Grid,
  Icon,
  Input,
  Menu,
  Button,
  Comment,
  Form,
  Header
} from 'semantic-ui-react'

import {HotKeys} from 'react-hotkeys'

var ws = new WebSocket('ws://localhost:8001/ws')
const activeChannel = {}

var state = {
  messages: [],
  activeChannel: '',
  channels: [],
  users: [],
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
    updateState(Object.assign({}, state, {
      users: Array.from(new Set(state.users.concat([
        data.User
      ]))),
    }))
  } else if (data.Type === 'channel')  {
    updateState(Object.assign({}, state, {
      channels: Array.from(new Set(state.channels.concat([
        data.Channel
      ]))),
    }))
  }
})

ws.addEventListener('close', function () {
  ws = new WebSocket('ws://localhost:8001/ws')
});

function updateState (newState) {
  window.state = state = newState
  redraw(state)
}

function setActiveChannel (channel) {
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

const Feed = ({messages, activeChannel}) => {
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
    ).map((message) => {
      const differentUser = message.Username !== previousUsername
      previousUsername = message.Username
      return <Comment>
        {
          differentUser
            ? <Comment.Avatar as='a' src={'https://robohash.org/' + message.Username} />
            : <Comment.Avatar />
        }
        <Comment.Content>
          { differentUser ? <Comment.Author as='a'>{message.Username}</Comment.Author> : null }
          { differentUser
              ?<Comment.Metadata>
                <span>Today at 5:42PM</span>
              </Comment.Metadata>
              : null }
          <Comment.Text>{message.Text}</Comment.Text>
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

  render({channels, activeChannel}) {
    return <Menu vertical fluid >
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
          return !this.state.filter || c.ID.indexOf(this.state.filter) >= 0
        }).map(c => (
          <Menu.Item onClick={() => setActiveChannel(c)} active={c == activeChannel} >
            {c.Name || c.ID}
          </Menu.Item>
        ))
      }
    </Menu>
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
      handlers={{
        'focusChannelSearch': () => this.channels.focusChannelSearch()
      }}>
      <div>
        <Grid padded columns={2} >
          <Grid.Row>
            <Grid.Column width={3}>
              <Channels
                ref={(el) => this.channels = el}
                channels={state.channels.concat(state.users)}
                activeChannel={state.activeChannel} />
            </Grid.Column>
            <Grid.Column width={13}>
              <Feed messages={state.messages} activeChannel={state.activeChannel} />
              <MessageInput activeChannel={state.activeChannel} />
            </Grid.Column>
          </Grid.Row>
        </Grid>
      </div>
    </HotKeys>
  }
}

function redraw ({messages}) {
  const chat = document.querySelector('#chat')
  render((<App state={state}/>), chat, chat.lastChild)
}

redraw(state)
