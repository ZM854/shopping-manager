import { NavLink } from "react-router-dom";
import ProfileIcon from "../../components/UI/svg/ProfileIcon/ProfileIcon";
import ListIcon from "../../components/UI/svg/ListIcon/ListIcon";
import cls from "./BottomNavigation.module.css";

const BottomNavigation = () => {
  return (
    <nav className={cls.navigation}>
      <NavLink
        to="/"
        end
        className={({ isActive }) =>
          isActive ? `${cls.link} ${cls.active}` : cls.link
        }
      >
        <ListIcon className={cls.icon} />

        <span>Покупки</span>
      </NavLink>

      <NavLink
        to="/profile"
        className={({ isActive }) =>
          isActive ? `${cls.link} ${cls.active}` : cls.link
        }
      >
        <ProfileIcon className={cls.icon} />

        <span>Профиль</span>
      </NavLink>
    </nav>
  );
};

export default BottomNavigation;
