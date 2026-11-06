<script lang="ts">
  import face from './gaffer-companion.png'
  import seated from './gaffer-companion-sitting.png'
  import './gafferCompanionAvatar.css'
  import {gafferAnimations} from './gafferMotion'
  let {size=32,thinking=false,seed=0,sitting=false}:{size?:number;thinking?:boolean;seed?:number;sitting?:boolean}=$props()
</script>
<span class="companion-face" class:thinking class:sitting class:still={!$gafferAnimations} data-greeting={seed%3} style:width={`${size}px`} style:height={`${size}px`} aria-hidden="true"><img src={sitting?seated:face} alt="" width="64" height="64"/></span>
